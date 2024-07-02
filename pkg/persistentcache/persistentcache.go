// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019-present Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package persistentcache

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
)

// Invalid characters to clean up
var invalidChars = regexp.MustCompile("[^a-zA-Z0-9_-]")

// Return a file where to store the data. We split the key by ":", using the
// first prefix as directory, if present. This is useful for integrations, which
// use the check_id formed with $check_name:$hash
func getFileForKey(key string) (string, error) {
	parent := config.Datadog().GetString("run_path")
	paths := strings.SplitN(key, ":", 2)
	cleanedPath := invalidChars.ReplaceAllString(paths[0], "")
	if len(paths) == 1 {
		// If there is no colon, just return the key
		return filepath.Join(parent, cleanedPath), nil
	}
	// Otherwise, create the directory with a prefix
	err := os.MkdirAll(filepath.Join(parent, cleanedPath), 0700)
	if err != nil {
		return "", err
	}
	cleanedFile := invalidChars.ReplaceAllString(paths[1], "")
	return filepath.Join(parent, cleanedPath, cleanedFile), nil
}

// Write stores data on disk in the run directory.
func Write(key, value string) error {
	path, err := getFileForKey(key)
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(value), 0600)
}

// Read returns a value previously stored, or the empty string.
func Read(key string) (string, error) {
	path, err := getFileForKey(key)
	if err != nil {
		return "", err
	}
	_, err = os.Stat(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(content), nil
}
