// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package events

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/process/events/model"
)

// AssertProcessEvents compares two ProcessEvents. Two events can't be compared using directly assert.Equal
// due to the embedded time fields
func AssertProcessEvents(t *testing.T, expected, actual *model.ProcessEvent) {
	t.Helper()

	assert.Equal(t, expected.EventType, actual.EventType)
	assert.WithinDuration(t, expected.CollectionTime, actual.CollectionTime, 0)
	assert.Equal(t, expected.Pid, actual.Pid)
	assert.Equal(t, expected.ContainerID, actual.ContainerID)
	assert.Equal(t, expected.Ppid, actual.Ppid)
	assert.Equal(t, expected.UID, actual.UID)
	assert.Equal(t, expected.GID, actual.GID)
	assert.Equal(t, expected.Username, actual.Username)
	assert.Equal(t, expected.Group, actual.Group)
	assert.Equal(t, expected.Exe, actual.Exe)
	assert.Equal(t, expected.Cmdline, actual.Cmdline)
	assert.WithinDuration(t, expected.ForkTime, actual.ForkTime, 0)
	assert.WithinDuration(t, expected.ExecTime, actual.ExecTime, 0)
	assert.WithinDuration(t, expected.ExitTime, actual.ExitTime, 0)
	assert.Equal(t, expected.ExitCode, actual.ExitCode)
}
