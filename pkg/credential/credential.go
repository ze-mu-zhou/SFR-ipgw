// Package credential stores passwords in the current Windows user's credential vault.
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
	var b [16]byte
	if _, e := rand.Read(b[:]); e != nil {
		return "", e
	}
	return fmt.Sprintf("%s/%x", Reference(username), b), nil
}
