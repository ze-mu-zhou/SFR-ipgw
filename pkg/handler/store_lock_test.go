package handler

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/ze-mu-zhou/SFR-ipgw/pkg/model"
)

func TestUpdateConfigReloadsBeforeChange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	a, _ := NewStoreHandler(path)
	a.Config = &model.Config{Accounts: []*model.Account{{Username: "alice", CredentialRef: "old"}}}
	if err := a.Persist(); err != nil {
		t.Fatal(err)
	}
	b, _ := NewStoreHandler(path)
	if err := b.Load(); err != nil {
		t.Fatal(err)
	}
	vault := map[string]bool{"old": true, "new": true}
	previousDelete := deleteCredential
	t.Cleanup(func() { deleteCredential = previousDelete })
	deleteCredential = func(ref string) error {
		delete(vault, ref)
		// The lock must remain held during cleanup, not just during rename.
		called := false
		_, err := b.UpdateConfig(func(*model.Config) error { called = true; return nil })
		if err == nil || called {
			t.Error("another update entered during credential cleanup")
		}
		return nil
	}
	warning, err := a.UpdateConfig(func(c *model.Config) error {
		c.GetAccount("alice").CredentialRef = "new"
		return c.AddAccount("bob", "")
	})
	if warning != nil || err != nil {
		t.Fatalf("warning=%v error=%v", warning, err)
	}
	// b still has the old in-memory reference, but must change the latest disk state.
	warning, err = b.UpdateConfig(func(c *model.Config) error {
		if c.GetAccount("alice").CredentialRef != "new" || c.GetAccount("bob") == nil {
			t.Fatal("change callback received stale configuration")
		}
		c.SetDefaultAccount("alice")
		return nil
	})
	if warning != nil || err != nil {
		t.Fatalf("warning=%v error=%v", warning, err)
	}
	if err := a.Load(); err != nil {
		t.Fatal(err)
	}
	if a.Config.DefaultAccount != "alice" || a.Config.GetAccount("bob") == nil || !vault[a.Config.GetAccount("alice").CredentialRef] {
		t.Fatal("stale writer lost changes or restored a deleted credential")
	}
}

func TestUpdateConfigReloadFailureHasNoSideEffects(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	store, _ := NewStoreHandler(path)
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	original := store.Config
	if err := os.WriteFile(path, []byte("invalid JSON"), 0600); err != nil {
		t.Fatal(err)
	}
	called := false
	if _, err := store.UpdateConfig(func(*model.Config) error { called = true; return nil }); err == nil || called || store.Config != original {
		t.Fatal("reload failure reached the change callback or mutated memory")
	}
	lock, err := lockConfig(path)
	if err != nil {
		t.Fatalf("failed transaction leaked lock: %v", err)
	}
	lock.Close()
}

// This helper is only run by the subprocess tests below.
func TestConfigLockHelper(t *testing.T) {
	path := os.Getenv("IPGW_TEST_LOCK_PATH")
	if path == "" {
		t.Skip("subprocess helper")
	}
	lock, err := lockConfig(path)
	if os.Getenv("IPGW_TEST_LOCK_MODE") == "busy" {
		if err == nil {
			lock.Close()
			t.Fatal("acquired a lock held by another process")
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if os.Getenv("IPGW_TEST_LOCK_MODE") == "hold" {
		fmt.Println("locked")
		time.Sleep(time.Minute) // parent kills the process to test crash recovery
	}
}

func lockHelperCommand(t *testing.T, path, mode string) *exec.Cmd {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestConfigLockHelper$")
	cmd.Env = append(os.Environ(), "IPGW_TEST_LOCK_PATH="+path, "IPGW_TEST_LOCK_MODE="+mode)
	return cmd
}

func TestConfigLockAcrossProcesses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	lock, err := lockConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if output, err := lockHelperCommand(t, path, "busy").CombinedOutput(); err != nil {
		t.Fatalf("lock was not exclusive: %v\n%s", err, output)
	}
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".lock"); err != nil {
		t.Fatalf("stable sidecar was removed: %v", err)
	}
	if output, err := lockHelperCommand(t, path, "acquire").CombinedOutput(); err != nil {
		t.Fatalf("closed handle did not release lock: %v\n%s", err, output)
	}
}

func TestConfigLockReleasedAfterProcessExit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	cmd := lockHelperCommand(t, path, "hold")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if cmd.ProcessState == nil {
			cmd.Process.Kill()
			cmd.Wait()
		}
	}()
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || line != "locked\n" {
		// Wait before reading stderr, which os/exec writes asynchronously.
		cmd.Process.Kill()
		cmd.Wait()
		t.Fatalf("helper did not acquire lock: %q %v %s", line, err, stderr.String())
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	lock, err := lockConfig(path)
	if err != nil {
		t.Fatalf("terminated process left a stale lock: %v", err)
	}
	lock.Close()
}
