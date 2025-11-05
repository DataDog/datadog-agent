// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package sharedlibrary

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
)

/*
#include "ffi.h"

void noop_run_function(char *check_id, char *init_config, char *instance_config, const aggregator_t *aggregator, const char **error) {
	// do nothing
}

library_t get_mock_library(void) {
	// only the symbol is required to run the check, so the library handle can be set to NULL
	library_t library = { NULL, noop_run_function };
	return library;
}
*/
import "C"

func testRunCheck(t *testing.T) {
	check, err := NewSharedLibraryFakeCheck(aggregator.NewNoOpSenderManager())
	if !assert.Nil(t, err) {
		return
	}

	err = check.runCheckImpl(false)
	assert.Nil(t, err)
}

func testRunCheckWithNullSymbol(t *testing.T) {
	check, err := NewSharedLibraryFakeCheck(aggregator.NewNoOpSenderManager())
	if !assert.Nil(t, err) {
		return
	}

	// set the symbol handle to NULL
	check.lib.run = nil

	err = check.runCheckImpl(false)
	assert.Error(t, err, "pointer to shared library 'Run' symbol is NULL")
}

func testCancelCheck(t *testing.T) {
	check, err := NewSharedLibraryFakeCheck(aggregator.NewNoOpSenderManager())
	if !assert.Nil(t, err) {
		return
	}

	check.Cancel()
	assert.True(t, check.cancelled)

	err = check.runCheckImpl(false)
	assert.Error(t, err, "check %s is already cancelled", check.name)
}

// NewSharedLibraryFakeCheck creates a fake SharedLibraryCheck
func NewSharedLibraryFakeCheck(senderManager sender.SenderManager) (*Check, error) {
	c, err := NewSharedLibraryCheck(senderManager, "fake_check", newSharedLibraryLoader("fake/library/folder/path"), getMockLibrary())

	// Remove check finalizer that may trigger race condition while testing
	if err == nil {
		runtime.SetFinalizer(c, nil)
	}

	return c, err
}

func getMockLibrary() library {
	return (library)(C.get_mock_library())
}
