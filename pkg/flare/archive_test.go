// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package flare

import (
	"os"
	"testing"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/stretchr/testify/assert"
)

func TestCreateArchive(t *testing.T) {
	common.SetupConfig("./test")
	config.Datadog.Set("confd_path", "./test/confd")
	config.Datadog.Set("log_file", "./test/logs/agent.log")
	zipFilePath := mkFilePath()
	filePath, err := createArchive(zipFilePath, true, SearchPaths{})

	assert.Nil(t, err)
	assert.Equal(t, zipFilePath, filePath)

	if _, err := os.Stat(zipFilePath); os.IsNotExist(err) {
		assert.Fail(t, "The Zip File was not created")
	} else {
		os.Remove(zipFilePath)
	}
}

func TestCreateArchiveBadConfig(t *testing.T) {
	/**
		The zipfile should be created even if there is no config file.
	**/

	common.SetupConfig("")
	zipFilePath := mkFilePath()
	filePath, err := createArchive(zipFilePath, true, SearchPaths{})

	assert.Nil(t, err)
	assert.Equal(t, zipFilePath, filePath)

	if _, err := os.Stat(zipFilePath); os.IsNotExist(err) {
		assert.Fail(t, "The Zip File was not created")
	} else {
		os.Remove(zipFilePath)
	}
}
