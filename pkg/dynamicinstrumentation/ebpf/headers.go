// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ebpf

import (
	"embed"
	"os"
	"path/filepath"
)

//go:embed headers
var headersFS embed.FS

// loadHeadersToTmpfs takes Go DIs needed header files from the embedded filesystem
// and loads them into the systems tmpfs so clang can find them
//
// The returned string is the directory path of the headers directory in tmpfs
func loadHeadersToTmpfs(directory string) (string, error) {
	fs, err := headersFS.ReadDir("headers")
	if err != nil {
		return "", err
	}

	tmpHeaderDir, err := os.MkdirTemp(directory, "dd-di-bpf-headers")
	if err != nil {
		return "", err
	}

	for _, entry := range fs {
		content, err := headersFS.ReadFile(filepath.Join("headers", entry.Name()))
		if err != nil {
			return "", err
		}
		err = os.WriteFile(filepath.Join(tmpHeaderDir, entry.Name()), content, 0644)
		if err != nil {
			return "", err
		}
	}
	return tmpHeaderDir, nil
}
