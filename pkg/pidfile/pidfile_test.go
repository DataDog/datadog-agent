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

func TestIsProcess(t *testing.T) {
	assert.True(t, isProcess(os.Getpid()))
}
