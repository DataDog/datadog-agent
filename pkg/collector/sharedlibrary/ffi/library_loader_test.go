// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build sharedlibrarycheck

package ffi

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
)

func TestOpen_NonExistentFile(t *testing.T) {
	loader := NewSharedLibraryLoader(t.TempDir())

	_, err := loader.Open("/path/that/does/not/exist.so")
	assert.Error(t, err)
}

func TestRun_NullLibraryPointer(t *testing.T) {
	loader := NewSharedLibraryLoader(t.TempDir())
	senderManager := aggregator.NewNoOpSenderManager()

	err := loader.Run(nil, "", "", "", "{}", senderManager)
	assert.EqualError(t, err, "Pointer to 'Library' struct is NULL")

	_, err = loader.Version(nil)
	assert.EqualError(t, err, "Pointer to 'Library' struct is NULL")

	err = loader.Close(nil)
	assert.EqualError(t, err, "Pointer to 'Library' struct is NULL")
}

func TestRun_LibraryWithNullSymbols(t *testing.T) {
	loader := NewSharedLibraryLoader(t.TempDir())
	senderManager := aggregator.NewNoOpSenderManager()

	lib := NewLibraryWithNullSymbols()

	err := loader.Run(lib, "", "", "", "{}", senderManager)
	assert.EqualError(t, err, "Failed to run check: pointer to 'check_run' symbol of the shared library is NULL")

	_, err = loader.Version(lib)
	assert.EqualError(t, err, "Failed to get version: pointer to 'Version' symbol of the shared library is NULL")
}
