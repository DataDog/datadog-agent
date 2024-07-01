// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && test

// Package testutil provides utilities for testing the process monitor
package testutil

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
	procmon "github.com/DataDog/datadog-agent/pkg/process/monitor"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// RegisterProcessMonitorEventConsumer registers the process monitor consumer to an EventMonitor
func RegisterProcessMonitorEventConsumer(t *testing.T, evm *eventmonitor.EventMonitor) {
	procmonconsumer, err := procmon.NewProcessMonitorEventConsumer(evm)
	require.NoError(t, err)
	evm.RegisterEventConsumer(procmonconsumer)
	log.Info("process monitoring test consumer initialized")
}
