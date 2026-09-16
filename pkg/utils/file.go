package utils

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func GetExecutablePathAndDir() (path, dir string, err error) {
	p, e := os.Executable()
	if e != nil {
		return "", "", e
	}
	path, err = filepath.Abs(p)
	return path, filepath.Dir(path), err
}

// Unzip rejects traversal, links, special files and existing files.
func Unzip(zipFile, destDir string) error {
	z, e := zip.OpenReader(zipFile)
	if e != nil {
		return e
	}
	defer z.Close()
	root, e := filepath.Abs(destDir)
	if e != nil {
		return e
	}
	if e = os.MkdirAll(root, 0700); e != nil {
		return e
	}
	if e = rejectSymlinks(root); e != nil {
		return e
	}
	var total uint64
	for _, f := range z.File {
		name := f.Name
		if name == "" || strings.HasPrefix(name, "/") || strings.ContainsAny(name, "\\:") || filepath.IsAbs(name) {
			return fmt.Errorf("压缩包路径不安全：%q", name)
		}
		target := filepath.Join(root, filepath.FromSlash(name))
		rel, e := filepath.Rel(root, target)
		if e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return fmt.Errorf("压缩包路径逃逸目标目录：%q", name)
		}
		if f.Mode()&os.ModeType != 0 && !f.FileInfo().IsDir() {
			return fmt.Errorf("不支持的压缩包条目：%q", name)
		}
		if f.UncompressedSize64 > 256<<20-total {
			return fmt.Errorf("压缩包超出 256 MiB 解压限制")
		}
		total += f.UncompressedSize64
		if e = rejectSymlinks(target); e != nil {
			return e
		}
		if f.FileInfo().IsDir() {
			if e = os.MkdirAll(target, 0700); e != nil {
				return e
			}
			continue
		}
		if e = os.MkdirAll(filepath.Dir(target), 0700); e != nil {
			return e
		}
		if e = extractFile(f, target); e != nil {
			return e
		}
	}
	return nil
}
func rejectSymlinks(path string) error {
	for {
		info, e := os.Lstat(path)
		if e != nil && !os.IsNotExist(e) {
			return e
		}
		if e == nil && info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("解压路径中存在符号链接：%s", path)
		}
		parent := filepath.Dir(path)
		if parent == path {
			return nil
		}
		path = parent
	}
}
func extractFile(f *zip.File, target string) error {
	in, e := f.Open()
	if e != nil {
		return e
	}
	defer in.Close()
	out, e := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
