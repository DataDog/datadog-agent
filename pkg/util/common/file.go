// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"io"
	"math/rand"
	"os"
	"path"
	"path/filepath"
	"time"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

// CopyFile atomically copies file path `src“ to file path `dst`.
func CopyFile(src, dst string) error {
	fi, err := os.Stat(src)
	if err != nil {
		return err
	}
	perm := fi.Mode()

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	tmp, err := os.CreateTemp(filepath.Dir(dst), "")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()

	_, err = io.Copy(tmp, in)
	if err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}

	err = tmp.Close()
	if err != nil {
		os.Remove(tmpName)
		return err
	}

	err = os.Chmod(tmpName, perm)
	if err != nil {
		os.Remove(tmpName)
		return err
	}

	err = os.Rename(tmpName, dst)
	if err != nil {
		os.Remove(tmpName)
		return err
	}

	return nil
}

// CopyFileAll calls CopyFile, but will create necessary directories for  `dst`.
func CopyFileAll(src, dst string) error {
	err := EnsureParentDirsExist(dst)
	if err != nil {
		return err
	}

	return CopyFile(src, dst)
}

// CopyDir copies directory recursively
func CopyDir(src, dst string) error {
	var (
		err     error
		fds     []os.DirEntry
		srcinfo os.FileInfo
	)

	if srcinfo, err = os.Stat(src); err != nil {
		return err
	}

	if err = os.MkdirAll(dst, srcinfo.Mode()); err != nil {
		return err
	}

	if fds, err = os.ReadDir(src); err != nil {
		return err
	}
	for _, fd := range fds {
		s := path.Join(src, fd.Name())
		d := path.Join(dst, fd.Name())

		if fd.IsDir() {
			err = CopyDir(s, d)
		} else {
			err = CopyFile(s, d)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// GetFileSize gets the file size
func GetFileSize(path string) (int64, error) {
	stat, err := os.Stat(path)

	if err != nil {
		return 0, err
	}

	return stat.Size(), nil
}

// GetFileModTime gets the modification time
func GetFileModTime(path string) (time.Time, error) {
	stat, err := os.Stat(path)

	if err != nil {
		return time.Time{}, err
	}

	return stat.ModTime(), nil
}

// EnsureParentDirsExist makes a path immediately available for
// writing by creating the necessary parent directories.
func EnsureParentDirsExist(p string) error {
	err := os.MkdirAll(filepath.Dir(p), os.ModePerm)
	if err != nil {
		return err
	}

	return nil
}
