// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package filesystem

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
)

// FileExists returns true if a file exists and is accessible, false otherwise
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// ReadLines reads a file line by line
func ReadLines(filename string) ([]string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return []string{""}, err
	}
	defer f.Close()

	var ret []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		ret = append(ret, scanner.Text())
	}
	return ret, scanner.Err()
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

// EnsureParentDirsExist makes a path immediately available for
// writing by creating the necessary parent directories.
func EnsureParentDirsExist(p string) error {
	err := os.MkdirAll(filepath.Dir(p), os.ModePerm)
	if err != nil {
		return err
	}

	return nil
}

// OpenFileForWriting opens a file for writing
func OpenFileForWriting(filePath string) (*os.File, *bufio.Writer, error) {
	f, err := os.OpenFile(filePath, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, nil, fmt.Errorf("error opening file %s: %v", filePath, err)
	}
	bufWriter := bufio.NewWriter(f)
	return f, bufWriter, nil
}
