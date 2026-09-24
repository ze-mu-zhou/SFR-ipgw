package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/credential"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/model"
	"os"
	"path/filepath"
	"strings"
)

var deleteCredential = credential.Delete

type StoreHandler struct {
	Path   string
	Config *model.Config
}

func NewStoreHandler(path string) (*StoreHandler, error) {
	p, e := getConfigPath(path)
	if e != nil {
		return nil, e
	}
	return &StoreHandler{Path: p}, nil
}
func getConfigPath(path string) (string, error) {
	if path != "" {
		return path, nil
	}
	home, e := os.UserHomeDir()
	if e != nil {
		return "", e
	}
	return filepath.Join(home, ".ipgw"), nil
}

// Persist replaces the complete configuration. Read-modify-write callers must
// use UpdateConfig, which reloads the latest state under the same lock.
func (h *StoreHandler) Persist() error {
	if h.Config == nil {
		return errors.New("未加载配置")
	}
	lock, err := lockConfig(h.Path)
	if err != nil {
		return err
	}
	defer lock.Close()
	return h.persistLocked()
}

func (h *StoreHandler) persistLocked() error {
	data, e := json.MarshalIndent(h.Config, "", "  ")
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(h.Path), ".ipgw-*")
	if e != nil {
		return e
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if e = f.Chmod(0600); e != nil {
		f.Close()
		return e
	}
	if _, e = f.Write(data); e != nil {
		f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	return os.Rename(tmp, h.Path)
}
func (h *StoreHandler) Load() error {
	data, e := os.ReadFile(h.Path)
	if os.IsNotExist(e) {
		h.Config = &model.Config{}
		return nil
	}
	if e != nil {
		return fmt.Errorf("加载配置失败：%w", e)
	}
	config := &model.Config{}
	if strings.TrimSpace(string(data)) != "" {
		if e = json.Unmarshal(data, config); e != nil {
			return fmt.Errorf("配置无效：%w", e)
		}
	}
	for _, a := range config.Accounts {
		if a == nil || a.Username == "" {
			return errors.New("配置中存在无效账号")
		}
	}
	h.Config = config
	return nil
}

// UpdateConfig 持有跨进程锁，重新加载最新配置后在副本上修改。
// 锁覆盖加载、修改、保存及凭据清理；change 必须基于传入的最新配置校验。
// 保存失败时保留旧配置和旧凭据，仅回收本次新建的凭据；
// 保存成功后才清理不再引用的旧凭据。
// warning 表示配置已经保存、但旧凭据清理失败；err 表示修改未提交。
func (h *StoreHandler) UpdateConfig(change func(*model.Config) error) (warning, err error) {
	if h.Config == nil {
		return nil, errors.New("未加载配置")
	}
	lock, err := lockConfig(h.Path)
	if err != nil {
		return nil, err
	}
	defer lock.Close()
	latest := &StoreHandler{Path: h.Path}
	if err := latest.Load(); err != nil {
		return nil, err
	}
	original, old := h.Config, latest.Config
	next := *old
	next.Accounts = make([]*model.Account, len(old.Accounts))
	for i, account := range old.Accounts {
		copyAccount := *account
		next.Accounts[i] = &copyAccount
	}
	err = change(&next)
	if err == nil {
		h.Config = &next
		err = h.persistLocked()
	}
	if err != nil {
		h.Config = original
		if cleanupErr := cleanupCredentials(&next, old); cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("清理本次新建凭据失败：%w", cleanupErr))
		}
		return nil, err
	}
	if cleanupErr := cleanupCredentials(old, &next); cleanupErr != nil {
		warning = fmt.Errorf("配置已保存，但清理旧凭据失败：%w", cleanupErr)
	}
	return warning, nil
}

// 仅删除 removed 中存在、retained 中已不再引用的凭据，并对共享引用去重。
func cleanupCredentials(removed, retained *model.Config) error {
	keep := make(map[string]bool)
	for _, account := range retained.Accounts {
		keep[account.CredentialRef] = true
	}
	var errs []error
	for _, account := range removed.Accounts {
		ref := account.CredentialRef
		if ref == "" || keep[ref] {
			continue
		}
		keep[ref] = true
		if err := deleteCredential(ref); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
