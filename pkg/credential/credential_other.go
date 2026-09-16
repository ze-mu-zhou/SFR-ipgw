//go:build !windows
// +build !windows

package credential

import "errors"

func Save(ref, username, password string) error {
	return errors.New("password storage is only supported by the Windows credential manager; use an interactive password")
}
func Load(ref string) (string, error) {
	return "", errors.New("Windows credentials are unavailable on this platform; use an interactive password")
}
