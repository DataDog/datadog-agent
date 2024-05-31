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

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
	emconfig "github.com/DataDog/datadog-agent/pkg/eventmonitor/config"
	secconfig "github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// PreStartCallback is a callback to register clients to the event monitor before starting it
type PreStartCallback func(t *testing.T, evm *eventmonitor.EventMonitor)

// StartEventMonitor creates and starts an event monitor for use in tests
func StartEventMonitor(t *testing.T, callback PreStartCallback) {
	if !sysconfig.ProcessEventDataStreamSupported() {
		t.Skip("Process event data stream not supported on this kernel")
	}
	emconfig := emconfig.NewConfig()
	secconfig, err := secconfig.NewConfig()
	require.NoError(t, err)

	// Needed for the socket creation to work
	require.NoError(t, os.MkdirAll("/opt/datadog-agent/run/", 0755))

	opts := eventmonitor.Opts{}
	evm, err := eventmonitor.NewEventMonitor(emconfig, secconfig, opts, optional.NewNoneOption[workloadmeta.Component]())
	require.NoError(t, err)
	require.NoError(t, evm.Init())
	callback(t, evm)
	require.NoError(t, evm.Start())
	t.Cleanup(evm.Close)
}
