// Package credential stores passwords in the operating system's credential
// store: Windows Credential Manager on Windows, and the system keyring
// (macOS Keychain or a Linux Secret Service such as GNOME Keyring) elsewhere.
package credential

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
)

func Reference(username string) string {
	return fmt.Sprintf("ipgw/account/%x", sha256.Sum256([]byte(username)))
}

func NewReference(username string) (string, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/%x", Reference(username), random), nil
}
