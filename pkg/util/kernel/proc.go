// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package kernel

import (
	"bufio"
	"bytes"
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
