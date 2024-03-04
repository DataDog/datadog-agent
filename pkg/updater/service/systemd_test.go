// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows
// +build !windows

package service

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setTestFifoPaths(testName string) {
	updaterTestID := time.Now().UnixNano()
	inFifoPath = fmt.Sprintf("/tmp/updater_%s_%v_in.fifo", testName, updaterTestID)
	outFifoPath = fmt.Sprintf("/tmp/updater_%s_%v_out.fifo", testName, updaterTestID)
}

func TestScriptRunnerBootAndCleanup(t *testing.T) {
	setTestFifoPaths("boot_clean")

	// installing fake fifo files to assert cleanup at newScriptRunner
	f, err := os.Create(inFifoPath)
	assert.Nil(t, err)
	assert.Nil(t, f.Close())
	f, err = os.Create(outFifoPath)
	assert.Nil(t, err)
	assert.Nil(t, f.Close())

	s, err := newScriptRunner()
	assert.Nil(t, err)
	require.NotNil(t, s)
	defer s.close()

	fileInfo, err := os.Stat(inFifoPath)
	assert.Equal(t, fileInfo.Mode()&os.ModeNamedPipe, os.ModeNamedPipe)
	assert.Nil(t, err)

	fileInfo, err = os.Stat(outFifoPath)
	assert.Equal(t, fileInfo.Mode()&os.ModeNamedPipe, os.ModeNamedPipe)
	assert.Nil(t, err)

	s.close()
	_, err = os.Stat(inFifoPath)
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(outFifoPath)
	assert.True(t, os.IsNotExist(err))
}
