// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && test

// Package testutil provides utilities for using the event monitor in tests
package testutil

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
	"github.com/DataDog/datadog-agent/pkg/eventmonitor/consumers"

	eventtestutil "github.com/DataDog/datadog-agent/pkg/eventmonitor/testutil"
)

const defaultChanSize = 100

// NewTestProcessConsumer creates a new ProcessConsumer for testing, registering itself with an event monitor
// created for testing. This function should be called in tests that require a ProcessConsumer.
func NewTestProcessConsumer(t *testing.T) *consumers.ProcessConsumer {
	var pc *consumers.ProcessConsumer
	eventtestutil.StartEventMonitor(t, func(t *testing.T, evm *eventmonitor.EventMonitor) {
		var err error
		eventTypes := []consumers.ProcessConsumerEventTypes{consumers.ExecEventType, consumers.ExitEventType}
		pc, err = consumers.NewProcessConsumer("test", defaultChanSize, eventTypes, evm)
		require.NoError(t, err, "failed to create process consumer")
	})

	return pc
}
