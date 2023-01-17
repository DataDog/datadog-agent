// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package util

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetRootNSPID(t *testing.T) {
	t.Run("HOST_PROC not set", func(t *testing.T) {
		pid, err := GetRootNSPID()
		assert.Nil(t, err)
		assert.Equal(t, os.Getpid(), pid)
	})

	t.Run("HOST_PROC set but not available", func(t *testing.T) {
		t.Setenv("HOST_PROC", "/foo/bar")
		pid, err := GetRootNSPID()
		assert.NotNil(t, err)
		assert.Equal(t, 0, pid)
	})
}
