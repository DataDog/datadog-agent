// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.

package persistentcache

import (
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func getFileForKey(key string) string {
	// Make it Windows compliant
	cleanedKey := strings.Replace(key, ":", "_", -1)
	return filepath.Join(config.Datadog.GetString("var_path"), cleanedKey)
}

// Write stores data on disk in the var directory.
func Write(key, value string) error {
	path := getFileForKey(key)
	return ioutil.WriteFile(path, []byte(value), 0600)
}

// Read returns a value previously stored, or the empty string.
func Read(key string) string {
	path := getFileForKey(key)
	content, err := ioutil.ReadFile(path)
	if err != nil {
		log.Debugf("Error reading data file: %v", err)
		return ""
	}
	return string(content)
}
