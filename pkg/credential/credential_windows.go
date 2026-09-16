//go:build windows
// +build windows

package credential

import (
	"errors"
	"fmt"
	"runtime"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

var advapi = syscall.NewLazyDLL("advapi32.dll")
var credWrite = advapi.NewProc("CredWriteW")
var credRead = advapi.NewProc("CredReadW")
var credFree = advapi.NewProc("CredFree")
var credDelete = advapi.NewProc("CredDeleteW")

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
		return errors.New("无效的凭据引用")
	}
	user, e := syscall.UTF16PtrFromString(username)
	if e != nil {
		return errors.New("无效的账号名")
	}
	data := utf16.Encode([]rune(password))
	if len(data) == 0 || len(data)*2 > 2560 {
		return errors.New("密码长度超出 Windows 凭据管理器的支持范围")
	}
	c := winCredential{Type: 1, TargetName: target, UserName: user, BlobSize: uint32(len(data) * 2), Blob: (*byte)(unsafe.Pointer(&data[0])), Persist: 2}
	ok, _, _ := credWrite.Call(uintptr(unsafe.Pointer(&c)), 0)
	runtime.KeepAlive(c)
	runtime.KeepAlive(data)
	for i := range data {
		data[i] = 0
	}
	if ok == 0 {
		return errors.New("写入 Windows 凭据管理器失败")
	}
	return nil
}
func Load(ref string) (string, error) {
	target, e := syscall.UTF16PtrFromString(ref)
	if e != nil {
		return "", errors.New("无效的凭据引用")
	}
	var c *winCredential
	ok, _, _ := credRead.Call(uintptr(unsafe.Pointer(target)), 1, 0, uintptr(unsafe.Pointer(&c)))
	runtime.KeepAlive(target)
	if ok == 0 {
		return "", errors.New("无法读取已保存的 Windows 凭据，请重新输入密码")
	}
	defer credFree.Call(uintptr(unsafe.Pointer(c)))
	if c.BlobSize%2 != 0 || c.BlobSize > 2560 || c.Blob == nil {
		return "", errors.New("已保存的凭据无效")
	}
	data := unsafe.Slice((*uint16)(unsafe.Pointer(c.Blob)), int(c.BlobSize/2))
	return string(utf16.Decode(data)), nil
}

// Delete 从凭据管理器删除条目；条目不存在时视为成功（幂等）。
func Delete(ref string) error {
	target, e := syscall.UTF16PtrFromString(ref)
	if e != nil {
		return errors.New("无效的凭据引用")
	}
	ok, _, lastErr := credDelete.Call(uintptr(unsafe.Pointer(target)), 1, 0)
	runtime.KeepAlive(target)
	if ok == 0 {
		const errorNotFound = syscall.Errno(1168)
		if lastErr == errorNotFound {
			return nil
		}
		return fmt.Errorf("删除 Windows 凭据失败：%w", lastErr)
	}
	return nil
}
