// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build sharedlibrarycheck

package sharedlibrarycheck

import (
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check/defaults"
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

func TestConfigureCollectionInterval(t *testing.T) {
	tests := []struct {
		name           string
		instanceConfig string
		expected       time.Duration
	}{
		{"explicit zero schedules a one-shot check", "min_collection_interval: 0", 0},
		{"positive value is interpreted as seconds", "min_collection_interval: 15", 15 * time.Second},
		{"unset keeps the default interval", "{}", defaults.DefaultCheckInterval},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			check, err := newFakeCheck(aggregator.NewNoOpSenderManager())
			require.NoError(t, err)

			err = check.Configure(check.senderManager, 0, integration.Data(tc.instanceConfig), nil, "test", "test")
			require.NoError(t, err)

			assert.Equal(t, tc.expected, check.Interval())
		})
	}
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
