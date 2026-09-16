package model

import (
	"errors"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/credential"
	"github.com/ze-mu-zhou/SFR-ipgw/pkg/utils"
)

type Account struct {
	Username          string `json:"username"`
	Password          string `json:"-"`
	Cookie            string `json:"-"`
	Secret            string `json:"-"`
	EncryptedPassword string `json:"encrypted_password,omitempty"`
	CredentialRef     string `json:"credential_ref,omitempty"`
}

func (a *Account) GetPassword() (string, error) {
	if a.Password != "" {
		return a.Password, nil
	}
	if a.CredentialRef != "" {
		return credential.Load(a.CredentialRef)
	}
	if a.EncryptedPassword == "" {
		return "", errors.New("no password stored; enter a password interactively")
	}
	p, e := utils.Decrypt(a.EncryptedPassword, []byte(a.Secret))
	if e != nil {
		return "", errors.New("legacy password decryption failed; check secret")
	}
	return p, nil
}

// SetPassword uses the OS vault; secret is retained only for source compatibility.
func (a *Account) SetPassword(password string, secret []byte) error {
	ref, err := credential.NewReference(a.Username)
	if err != nil {
		return err
	}
	if e := credential.Save(ref, a.Username, password); e != nil {
		return e
	}
	a.CredentialRef = ref
	a.Password = password
	a.EncryptedPassword = ""
	return nil
}
func (a *Account) String() string { return a.Username }
