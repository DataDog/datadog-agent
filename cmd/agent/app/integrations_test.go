// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build cpython

package app

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMoveConfigurationsFiles(t *testing.T) {
	srcFolder, _ := ioutil.TempDir("", "srcFolder")
	dstFolder, _ := ioutil.TempDir("", "dstFolder")
	defer os.RemoveAll(srcFolder)
	defer os.RemoveAll(dstFolder)
	yamlFiles := []string{"conf.yaml.example", "conf.yaml.default", "metrics.yaml", "auto_conf.yaml"}
	otherFile := "not_moved.txt"
	for _, filename := range append(yamlFiles, otherFile) {
		os.Create(filepath.Join(srcFolder, filename))
	}

	filesCreated, _ := ioutil.ReadDir(srcFolder)
	assert.Equal(t, 5, len(filesCreated))
	for _, file := range filesCreated {
		assert.Contains(t, append(yamlFiles, otherFile), file.Name())
	}

	moveConfigurationFiles(srcFolder, dstFolder)

	filesMoved, _ := ioutil.ReadDir(dstFolder)
	assert.Equal(t, 4, len(filesMoved))
	for _, file := range filesMoved {
		assert.Contains(t, yamlFiles, file.Name())
	}
	filesNotMoved, _ := ioutil.ReadDir(srcFolder)
	assert.Equal(t, 1, len(filesNotMoved))
	assert.Equal(t, otherFile, filesNotMoved[0].Name())
}
