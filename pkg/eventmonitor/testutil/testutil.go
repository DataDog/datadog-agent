// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package testutil provides utilities for using the event monitor in tests
package testutil

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
	emconfig "github.com/DataDog/datadog-agent/pkg/eventmonitor/config"
	secconfig "github.com/DataDog/datadog-agent/pkg/security/config"
	secmodule "github.com/DataDog/datadog-agent/pkg/security/module"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
)

// PreStartCallback is a callback to register clients to the event monitor before starting it
type PreStartCallback func(tb testing.TB, evm *eventmonitor.EventMonitor)

// StartEventMonitor creates and starts an event monitor for use in tests
func StartEventMonitor(tb testing.TB, callback PreStartCallback) {
	if !sysconfig.ProcessEventDataStreamSupported() {
		tb.Skip("Process event data stream not supported on this kernel")
	}
	emconfig := emconfig.NewConfig()
	secconfig, err := secconfig.NewConfig()
	require.NoError(tb, err)

	// disable the CWS part (similar to running USM/CNM only)
	secmodule.DisableRuntimeSecurity(secconfig)

	// Needed for the socket creation to work
	require.NoError(tb, os.MkdirAll("/opt/datadog-agent/run/", 0755))

	opts := eventmonitor.Opts{}
	evm, err := eventmonitor.NewEventMonitor(emconfig, secconfig, "test-hostname", opts)
	require.NoError(tb, err)
	require.NoError(tb, evm.Init())
	callback(tb, evm)
	require.NoError(tb, evm.Start())
	tb.Cleanup(evm.Close)
}
