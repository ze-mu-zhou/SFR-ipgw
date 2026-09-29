package utils

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func ExecutablePathAndDir() (path, dir string, err error) {
	executable, err := os.Executable()
	if err != nil {
		return "", "", err
	}
	path, err = filepath.Abs(executable)
	return path, filepath.Dir(path), err
}

// Unzip rejects traversal, links, special files and existing files.
func Unzip(zipFile, destDir string) error {
	archive, err := zip.OpenReader(zipFile)
	if err != nil {
		return err
	}
	defer archive.Close()
	root, err := filepath.Abs(destDir)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(root, 0700); err != nil {
		return err
	}
	if err = rejectSymlinks(root); err != nil {
		return err
	}
	var total uint64
	for _, entry := range archive.File {
		name := entry.Name
		if name == "" || strings.HasPrefix(name, "/") || strings.ContainsAny(name, "\\:") || filepath.IsAbs(name) {
			return fmt.Errorf("压缩包路径不安全：%q", name)
		}
		target := filepath.Join(root, filepath.FromSlash(name))
		rel, err := filepath.Rel(root, target)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return fmt.Errorf("压缩包路径逃逸目标目录：%q", name)
		}
		if entry.Mode()&os.ModeType != 0 && !entry.FileInfo().IsDir() {
			return fmt.Errorf("不支持的压缩包条目：%q", name)
		}
		if entry.UncompressedSize64 > 256<<20-total {
			return fmt.Errorf("压缩包超出 256 MiB 解压限制")
		}
		total += entry.UncompressedSize64
		if err = rejectSymlinks(target); err != nil {
			return err
		}
		if entry.FileInfo().IsDir() {
			if err = os.MkdirAll(target, 0700); err != nil {
				return err
			}
			continue
		}
		if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return err
		}
		if err = extractFile(entry, target); err != nil {
			return err
		}
	}
	return nil
}

func rejectSymlinks(path string) error {
	for {
		info, err := os.Lstat(path)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("解压路径中存在符号链接：%s", path)
		}
		parent := filepath.Dir(path)
		if parent == path {
			return nil
		}
		path = parent
	}
}

func extractFile(entry *zip.File, target string) error {
	in, err := entry.Open()
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
