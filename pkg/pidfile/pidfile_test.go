// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pidfile

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWritePID(t *testing.T) {
	dir := t.TempDir()

	pidFilePath := filepath.Join(dir, "this_should_be_created", "agent.pid")
	err := WritePID(pidFilePath)
	assert.NoError(t, err)
	data, err := os.ReadFile(pidFilePath)
	assert.NoError(t, err)
	pid, err := strconv.Atoi(string(data))
	assert.NoError(t, err)
	assert.Equal(t, pid, os.Getpid())
}

func TestWritePIDOverStaleFile(t *testing.T) {
	dir := t.TempDir()
	pidFilePath := filepath.Join(dir, "agent.pid")

	// write a stale PID (non-existent process)
	err := os.WriteFile(pidFilePath, []byte("9999999"), 0644)
	assert.NoError(t, err)

	// should succeed because the PID isn't running
	err = WritePID(pidFilePath)
	assert.NoError(t, err)
}

func TestWritePIDExistingRunningProcess(t *testing.T) {
	dir := t.TempDir()
	pidFilePath := filepath.Join(dir, "agent.pid")

	// write the current PID (a running process)
	err := os.WriteFile(pidFilePath, []byte(strconv.Itoa(os.Getpid())), 0644)
	assert.NoError(t, err)

	// should fail because the PID is still running
	err = WritePID(pidFilePath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Pidfile already exists")
}

func TestWritePIDMalformedContent(t *testing.T) {
	dir := t.TempDir()
	pidFilePath := filepath.Join(dir, "agent.pid")

	// write garbage content
	err := os.WriteFile(pidFilePath, []byte("not-a-number"), 0644)
	assert.NoError(t, err)

	// should succeed since content is not a valid PID
	err = WritePID(pidFilePath)
	assert.NoError(t, err)
}

func TestIsProcess(t *testing.T) {
	assert.True(t, isProcess(os.Getpid()))
}
