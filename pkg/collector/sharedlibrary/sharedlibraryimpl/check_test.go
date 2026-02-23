// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build sharedlibrarycheck

package sharedlibrarycheck

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/sharedlibrary/ffi"
)

func TestRunCheck(t *testing.T) {
	check, err := newFakeCheck(aggregator.NewNoOpSenderManager())
	require.NoError(t, err)

	err = check.runCheckImpl(false)
	assert.NoError(t, err)
}

func TestRunCheckWithNullSymbol(t *testing.T) {
	check, err := newFakeCheck(aggregator.NewNoOpSenderManager())
	require.NoError(t, err)

	// set all the symbol pointers to NULL
	check.lib = ffi.NewLibraryWithNullSymbols()

	err = check.runCheckImpl(false)
	assert.Error(t, err, "pointer to shared library 'Run' symbol is NULL")
}

func TestCancelCheck(t *testing.T) {
	check, err := newFakeCheck(aggregator.NewNoOpSenderManager())
	require.NoError(t, err)

	check.Cancel()
	require.True(t, check.cancelled)

	err = check.runCheckImpl(false)
	assert.Error(t, err, "check %s is already cancelled", check.name)
}

func newFakeCheck(senderManager sender.SenderManager) (*Check, error) {
	sharedLibraryLoader, err := ffi.NewSharedLibraryLoader("fake/library/folder/path")
	if err != nil {
		return nil, err
	}

	c, err := newCheck(senderManager, "fake_check", sharedLibraryLoader, ffi.GetNoopLibrary())

	// Remove check finalizer that may trigger race condition while testing
	if err == nil {
		runtime.SetFinalizer(c, nil)
	}

	return c, err
}
