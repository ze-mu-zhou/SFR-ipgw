package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/credential"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/model"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/utils"
	"os"
	"path/filepath"
	"strings"
)

var saveCredential = credential.Save
var loadCredential = credential.Load

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
func (h *StoreHandler) Persist() error {
	if h.Config == nil {
		return errors.New("no configuration loaded")
	}
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
		return fmt.Errorf("load config: %w", e)
	}
	config := &model.Config{}
	if strings.TrimSpace(string(data)) != "" {
		if e = json.Unmarshal(data, config); e != nil {
			return fmt.Errorf("invalid config: %w", e)
		}
	}
	for _, a := range config.Accounts {
		if a == nil || a.Username == "" {
			return errors.New("invalid account in configuration")
		}
	}
	h.Config = config
	return nil
}

// MigrateCredentials is explicitly invoked by the user. Decode all legacy
// passwords before writing, and only replace the configuration after every
// credential has been stored and read back successfully.
func (h *StoreHandler) MigrateCredentials(secret string) error {
	if h.Config == nil {
		return errors.New("no configuration loaded")
	}
	copyConfig := *h.Config
	copyConfig.Accounts = make([]*model.Account, len(h.Config.Accounts))
	passwords := map[int]string{}
	for i, a := range h.Config.Accounts {
		c := *a
		copyConfig.Accounts[i] = &c
		if a.EncryptedPassword != "" {
			p, e := utils.Decrypt(a.EncryptedPassword, []byte(secret))
			if e != nil || p == "" {
				return errors.New("legacy password decryption failed; configuration unchanged")
			}
			passwords[i] = p
		}
	}
	for i, p := range passwords {
		a := copyConfig.Accounts[i]
		ref, err := credential.NewReference(a.Username)
		if err != nil {
			return err
		}
		if e := saveCredential(ref, a.Username, p); e != nil {
			return e
		}
		saved, e := loadCredential(ref)
		if e != nil || saved != p {
			return errors.New("credential verification failed; configuration unchanged")
		}
		a.CredentialRef = ref
		a.EncryptedPassword = ""
	}
	old := h.Config
	h.Config = &copyConfig
	if e := h.Persist(); e != nil {
		h.Config = old
		return e
	}
	return nil
}
