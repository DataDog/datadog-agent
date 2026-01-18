// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package kernel

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// AllPidsProcs will return all pids under procRoot
func AllPidsProcs(procRoot string) ([]int, error) {
	f, err := os.Open(procRoot)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	dirs, err := f.Readdirnames(-1)
	if err != nil {
		return nil, err
	}

	pids := make([]int, 0, len(dirs))
	for _, name := range dirs {
		if pid, err := strconv.Atoi(name); err == nil {
			pids = append(pids, pid)
		}
	}
	return pids, nil
}

// WithAllProcs will execute `fn` for every pid under procRoot. `fn` is
// passed the `pid`. If `fn` returns an error the iteration aborts,
// returning the last error returned from `fn`.
func WithAllProcs(procRoot string, fn func(int) error) error {
	pids, err := AllPidsProcs(procRoot)
	if err != nil {
		return err
	}

	for _, pid := range pids {
		if err = fn(pid); err != nil {
			return err
		}
	}
	return nil
}

// scanNullString is a SplitFunc for a Scanner that returns each null-terminated
// string as a token. Receives the data from the scanner that's yet to be
// processed into tokens, and whether the scanner has reached EOF.
//
// Returns the number of bytes to advance the scanner, the token that was
// detected and an error in case of failure
func scanNullStrings(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	if i := bytes.IndexByte(data, '\x00'); i >= 0 {
		// We have a full null-terminated line.
		return i + 1, data[0:i], nil
	}
	// If we're at EOF, we have a final, non-terminated line. Return it.
	if atEOF {
		return len(data), data, nil
	}
	// Request more data.
	return 0, nil, nil
}

func getEnvVariableFromBuffer(reader io.Reader, envVar string) string {
	scanner := bufio.NewScanner(reader)
	scanner.Split(scanNullStrings)
	for scanner.Scan() {
		parts := strings.SplitN(scanner.Text(), "=", 2)
		if len(parts) != 2 {
			continue
		}

		if parts[0] == envVar {
			return parts[1]
		}
	}

	return ""
}

// GetProcessEnvVariable retrieves the given environment variable for the specified process ID, without
// loading the entire environment into memory. Will return an empty string if the variable is not found.
func GetProcessEnvVariable(pid int, procRoot string, envVar string) (string, error) {
	envPath := filepath.Join(procRoot, strconv.Itoa(pid), "environ")
	envFile, err := os.Open(envPath)
	if err != nil {
		return "", fmt.Errorf("cannot open %s: %w", envPath, err)
	}
	defer envFile.Close()

	return getEnvVariableFromBuffer(envFile, envVar), nil
}

// ErrMemFdFileNotFound is an error for when there was no file
// found for a given process.
var ErrMemFdFileNotFound = errors.New("memfd file not found")

// ReadMemFdFile reads a maximum amount of bytes from the memfd file at the given path.
// The path should be a full path to an fd (e.g., /proc/1234/fd/5).
func ReadMemFdFile(path string, memFdMaxSize int) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return nil, err
	}

	if !fileInfo.Mode().IsRegular() {
		return nil, fmt.Errorf("%s: not a regular file", path)
	}

	data, err := io.ReadAll(io.LimitReader(file, int64(memFdMaxSize+1)))
	if err != nil {
		return nil, err
	}

	if len(data) > memFdMaxSize {
		return nil, io.ErrShortBuffer
	}

	return data, nil
}

// GetProcessMemFdFile reads a maximum amount of bytes from the
// memFdFileName if it was found.
func GetProcessMemFdFile(pid int, procRoot string, memFdFileName string, memFdMaxSize int) ([]byte, error) {
	path, found := findMemFdFilePath(pid, procRoot, memFdFileName)
	if !found {
		return nil, ErrMemFdFileNotFound
	}

	return ReadMemFdFile(path, memFdMaxSize)
}

// findMemfdFilePath searches for the file in the process open file descriptors.
// In order to find the correct file, we need to iterate the list of files
// (named after file descriptor numbers) in /proc/$PID/fd and get the name from
// the target of the symbolic link.
//
// ```
// $ ls -l /proc/1750097/fd/
// total 0
// lrwx------ 1 foo foo 64 Aug 13 14:24 0 -> /dev/pts/6
// lrwx------ 1 foo foo 64 Aug 13 14:24 1 -> /dev/pts/6
// lrwx------ 1 foo foo 64 Aug 13 14:24 2 -> /dev/pts/6
// lrwx------ 1 foo foo 64 Aug 13 14:24 3 -> '/memfd:dd_process_inject_info.msgpac (deleted)'
// ```
func findMemFdFilePath(pid int, procRoot string, memFdFileName string) (string, bool) {
	fdsPath := filepath.Join(procRoot, strconv.Itoa(pid), "fd")
	// quick path, the shadow file is the first opened file by the process
	// unless there are inherited fds
	path := filepath.Join(fdsPath, "3")
	if isMemfdFilePath(path, memFdFileName) {
		return path, true
	}
	fdDir, err := os.Open(fdsPath)
	if err != nil {
		return "", false
	}
	defer fdDir.Close()

	fds, err := fdDir.Readdirnames(-1)
	if err != nil {
		// log.Warnf("failed to read %s: %s", fdsPath, err)
		return "", false
	}

	for _, fd := range fds {
		switch fd {
		case "0", "1", "2", "3":
			continue
		default:
			path := filepath.Join(fdsPath, fd)
			if isMemfdFilePath(path, memFdFileName) {
				return path, true
			}
		}
	}

	return "", false
}

func isMemfdFilePath(path, memfdFileName string) bool {
	name, err := os.Readlink(path)
	if err != nil {
		return false
	}

	return strings.HasPrefix(name, "/memfd:"+memfdFileName)
}

// ProcessExists returns true if the process exists in the procfs
func ProcessExists(pid int) bool {
	path := filepath.Join(HostProc(), strconv.Itoa(pid))
	if _, err := os.Stat(path); err != nil {
		return false
	}
	return true
}
