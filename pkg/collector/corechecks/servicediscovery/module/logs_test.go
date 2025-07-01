// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package module

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetLogFiles(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test-log-files")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	writeLogPath := filepath.Join(tempDir, "test.log")
	f, err := os.OpenFile(writeLogPath, os.O_CREATE|os.O_WRONLY, 0644)
	require.NoError(t, err)
	defer f.Close()

	readWriteLogPath := filepath.Join(tempDir, "readwrite.log")
	f, err = os.OpenFile(readWriteLogPath, os.O_CREATE|os.O_RDWR, 0644)
	require.NoError(t, err)
	defer f.Close()

	writeOtherPath := filepath.Join(tempDir, "test.log.txt")
	f, err = os.OpenFile(writeOtherPath, os.O_CREATE|os.O_WRONLY, 0644)
	require.NoError(t, err)
	defer f.Close()

	readLogPath := filepath.Join(tempDir, "read.log")
	f, err = os.Create(readLogPath)
	require.NoError(t, err)
	f.Close()
	f, err = os.Open(readLogPath)
	require.NoError(t, err)
	defer f.Close()

	writeLargePath := filepath.Join(tempDir, strings.Repeat("a", 128)+".log")
	f, err = os.OpenFile(writeLargePath, os.O_CREATE|os.O_WRONLY, 0644)
	require.NoError(t, err)
	defer f.Close()

	self := int32(os.Getpid())

	buf := make([]byte, readlinkBufferSize)
	openFilesInfo, err := getOpenFilesInfo(self, buf)
	require.NoError(t, err)

	logFiles := getLogFiles(self, openFilesInfo.logs)

	assert.Contains(t, logFiles, writeLogPath)
	assert.NotContains(t, logFiles, readWriteLogPath)
	assert.Contains(t, logFiles, writeLargePath)
	assert.NotContains(t, logFiles, writeOtherPath)
	assert.NotContains(t, logFiles, readLogPath)
}
