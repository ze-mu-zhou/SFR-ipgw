package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ze-mu-zhou/SFR-ipgw/pkg/credential"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/model"
)

var deleteCredential = credential.Delete

type StoreHandler struct {
	Path   string
	Config *model.Config
}

func NewStoreHandler(path string) (*StoreHandler, error) {
	resolved, err := configPath(path)
	if err != nil {
		return nil, err
	}
	return &StoreHandler{Path: resolved}, nil
}

func configPath(path string) (string, error) {
	if path != "" {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".ipgw"), nil
}

// persistLocked replaces the complete configuration without reloading it, so
// callers must hold the configuration lock. Use UpdateConfig, which reloads the
// latest state under the same lock before changing it.
func (s *StoreHandler) persistLocked() error {
	data, err := json.MarshalIndent(s.Config, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(s.Path), ".ipgw-*")
	if err != nil {
		return err
	}
	tmp := file.Name()
	defer os.Remove(tmp)
	if err = file.Chmod(0600); err != nil {
		file.Close()
		return err
	}
	if _, err = file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err = file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, s.Path)
}

func (s *StoreHandler) Load() error {
	data, err := os.ReadFile(s.Path)
	if os.IsNotExist(err) {
		s.Config = &model.Config{}
		return nil
	}
	if err != nil {
		return fmt.Errorf("加载配置失败：%w", err)
	}
	config := &model.Config{}
	if strings.TrimSpace(string(data)) != "" {
		if err = json.Unmarshal(data, config); err != nil {
			return fmt.Errorf("配置无效：%w", err)
		}
	}
	for _, account := range config.Accounts {
		if account == nil || account.Username == "" {
			return errors.New("配置中存在无效账号")
		}
	}
	s.Config = config
	return nil
}

// UpdateConfig 持有跨进程锁，重新加载最新配置后在副本上修改。
// 锁覆盖加载、修改、保存及凭据清理；change 必须基于传入的最新配置校验。
// 保存失败时保留旧配置和旧凭据，仅回收本次新建的凭据；
// 保存成功后才清理不再引用的旧凭据。
// warning 表示配置已经保存、但旧凭据清理失败；err 表示修改未提交。
func (s *StoreHandler) UpdateConfig(change func(*model.Config) error) (warning, err error) {
	if s.Config == nil {
		return nil, errors.New("未加载配置")
	}
	lock, err := lockConfig(s.Path)
	if err != nil {
		return nil, err
	}
	defer lock.Close()
	latest := &StoreHandler{Path: s.Path}
	if err := latest.Load(); err != nil {
		return nil, err
	}
	original, old := s.Config, latest.Config
	next := *old
	next.Accounts = make([]*model.Account, len(old.Accounts))
	for i, account := range old.Accounts {
		copyAccount := *account
		next.Accounts[i] = &copyAccount
	}
	err = change(&next)
	if err == nil {
		s.Config = &next
		err = s.persistLocked()
	}
	if err != nil {
		s.Config = original
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
