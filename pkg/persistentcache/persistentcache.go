// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.

package persistentcache

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
)

// Return a file where to store the data. We split the key by ":", using the
// first prefix as directory, if present. This useful for integrations, which
// use the check_id formed with $check_name:$hash
func getFileForKey(key string) (string, error) {
	parent := config.Datadog.GetString("run_path")
	paths := strings.SplitN(key, ":", 2)
	if len(paths) == 1 {
		// If there is no colon, just return the key
		return filepath.Join(parent, paths[0]), nil
	}
	// Otherwise, create the directory with a prefix
	err := os.MkdirAll(filepath.Join(parent, paths[0]), 0700)
	if err != nil {
		return "", err
	}
	// Make the file Windows compliant
	cleanedPath := strings.Replace(paths[1], ":", "_", -1)
	return filepath.Join(parent, paths[0], cleanedPath), nil
}

// Write stores data on disk in the run directory.
func Write(key, value string) error {
	path, err := getFileForKey(key)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(path, []byte(value), 0600)
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
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(content), nil
}
