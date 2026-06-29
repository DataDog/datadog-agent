// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build docker || (cri && containerd)

package configfilesdiscoveryimpl

import (
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
)

const maxConfigFileSize = 1024 * 1024 // 1MiB

func filterEnvVars(envEntries []string, names []string) map[string]string {
	wanted := make(map[string]struct{}, len(names))
	for _, name := range names {
		wanted[name] = struct{}{}
	}

	env := make(map[string]string)
	for _, entry := range envEntries {
		name, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if _, ok := wanted[name]; !ok {
			continue
		}
		env[name] = value
	}

	return env
}

func cleanContainerFilePath(filePath string) (string, error) {
	if filePath == "" {
		return "", errors.New("empty config file path")
	}
	if !path.IsAbs(filePath) {
		return "", fmt.Errorf("config file path %q is not absolute", filePath)
	}
	for _, elem := range strings.Split(filePath, "/") {
		if elem == ".." {
			return "", fmt.Errorf("config file path %q contains parent traversal", filePath)
		}
	}
	return path.Clean(filePath), nil
}

func readLimitedFileContent(r io.Reader, limit int) ([]byte, bool, error) {
	// Read one byte past the returned content limit so callers can distinguish
	// a file exactly at the limit from a larger file.
	content, err := io.ReadAll(io.LimitReader(r, int64(limit)+1))
	if err != nil {
		return nil, false, err
	}
	if len(content) <= limit {
		return content, false, nil
	}
	return content[:limit], true, nil
}
