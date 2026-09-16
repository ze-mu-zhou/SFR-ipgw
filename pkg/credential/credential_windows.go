//go:build windows
// +build windows

package credential

import (
	"errors"
	"runtime"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

var advapi = syscall.NewLazyDLL("advapi32.dll")
var credWrite = advapi.NewProc("CredWriteW")
var credRead = advapi.NewProc("CredReadW")
var credFree = advapi.NewProc("CredFree")

type winCredential struct {
	Flags, Type             uint32
	TargetName, Comment     *uint16
	LastWritten             syscall.Filetime
	BlobSize                uint32
	Blob                    *byte
	Persist, AttributeCount uint32
	Attributes              uintptr
	TargetAlias, UserName   *uint16
}

func Save(ref, username, password string) error {
	target, e := syscall.UTF16PtrFromString(ref)
	if e != nil {
		return errors.New("invalid credential reference")
	}
	user, e := syscall.UTF16PtrFromString(username)
	if e != nil {
		return errors.New("invalid account name")
	}
	data := utf16.Encode([]rune(password))
	if len(data) == 0 || len(data)*2 > 2560 {
		return errors.New("password length is not supported by Windows credential storage")
	}
	c := winCredential{Type: 1, TargetName: target, UserName: user, BlobSize: uint32(len(data) * 2), Blob: (*byte)(unsafe.Pointer(&data[0])), Persist: 2}
	ok, _, _ := credWrite.Call(uintptr(unsafe.Pointer(&c)), 0)
	runtime.KeepAlive(c)
	runtime.KeepAlive(data)
	for i := range data {
		data[i] = 0
	}
	if ok == 0 {
		return errors.New("Windows credential storage failed")
	}
	return nil
}
func Load(ref string) (string, error) {
	target, e := syscall.UTF16PtrFromString(ref)
	if e != nil {
		return "", errors.New("invalid credential reference")
	}
	var c *winCredential
	ok, _, _ := credRead.Call(uintptr(unsafe.Pointer(target)), 1, 0, uintptr(unsafe.Pointer(&c)))
	runtime.KeepAlive(target)
	if ok == 0 {
		return "", errors.New("stored Windows credential unavailable; enter password again")
	}
	defer credFree.Call(uintptr(unsafe.Pointer(c)))
	if c.BlobSize%2 != 0 || c.BlobSize > 2560 || c.Blob == nil {
		return "", errors.New("invalid stored credential")
	}
	data := unsafe.Slice((*uint16)(unsafe.Pointer(c.Blob)), int(c.BlobSize/2))
	return string(utf16.Decode(data)), nil
}
