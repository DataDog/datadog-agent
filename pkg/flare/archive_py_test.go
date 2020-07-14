// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build python

package flare

import (
	"archive/zip"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/config"
)

func TestPyHeapProfile(t *testing.T) {
	assert := assert.New(t)

	common.SetupConfig("./test")
	mockConfig := config.Mock()
	mockConfig.Set("memtrack_enabled", true)
	zipFilePath := getArchivePath()
	opts := InitOptions(true, true)
	filePath, err := createArchive(zipFilePath, opts, SearchPaths{}, "")
	defer os.Remove(zipFilePath)

	assert.Nil(err)
	assert.Equal(zipFilePath, filePath)

	// asserts that it as indeed created a permissions.log file
	z, err := zip.OpenReader(zipFilePath)
	assert.NoError(err, "opening the zip shouldn't pop an error")

	ok := false
	for _, f := range z.File {
		if f.Name == "profile" {
			dir, err := f.Open()
			assert.Nil(err)
		}
	}
	assert.True(ok, "a profile directory should have been appended to the zip")
}

func rtLoaderEnabled() bool {
	return true
}
