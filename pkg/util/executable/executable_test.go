// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows || darwin || linux

package executable

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolvePath(t *testing.T) {
	testProgram := "ls"
	if runtime.GOOS == "windows" {
		testProgram = "dir"
	}

	actualPath, err := ResolvePath(testProgram)
	if !assert.Nil(t, err) {
		return
	}

	if !assert.NotEmpty(t, actualPath) {
		return
	}

	if _, err := os.Stat(actualPath); os.IsNotExist(err) {
		assert.FailNowf(t, "Resolved path '%s' does not exist!", actualPath)
	}
}

func TestResolvePathIsAbsolute(t *testing.T) {
	testProgram := "ls"
	if runtime.GOOS == "windows" {
		testProgram = "dir"
	}

	actualPath, err := ResolvePath(testProgram)
	if !assert.Nil(t, err) {
		return
	}

	absPath, err := filepath.Abs(actualPath)
	if !assert.Nil(t, err) {
		return
	}

	assert.Equal(t, absPath, actualPath)
}

func TestResolvePathFailure(t *testing.T) {
	testProgram := "badprogramname"

	_, err := ResolvePath(testProgram)
	if !assert.NotNil(t, err) {
		return
	}
}
