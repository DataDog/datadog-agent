// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build sharedlibrarycheck && test

package sharedlibrarycheck

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/sharedlibrary/ffi"
)

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
	check.lib.Run = nil

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
	c, err := NewSharedLibraryCheck(senderManager, "fake_check", ffi.NewSharedLibraryLoader("fake/library/folder/path"), ffi.GetNoopLibrary())

	// Remove check finalizer that may trigger race condition while testing
	if err == nil {
		runtime.SetFinalizer(c, nil)
	}

	return c, err
}
