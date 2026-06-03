// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package jobmetadata

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRecordFromControlEventUpdate(t *testing.T) {
	now := time.Unix(100, 0)

	record, consumed, err := recordFromEventAt(" container-a ", ControlEventTitle, ControlEventSourceType, "start", []string{
		"team:ml",
		"gpu_job_id:job-a",
		"team:ml",
		"host:spoofed",
		"dd.internal.foo:bar",
		"missing_colon",
	}, time.Second, now)
	require.NoError(t, err)
	require.True(t, consumed)
	require.Equal(t, "container-a", record.ContainerID)
	require.Equal(t, ActionUpdate, record.Action)
	require.Equal(t, "job-a", record.JobID)
	require.Equal(t, []string{"gpu_job_id:job-a", "team:ml"}, record.Tags)
	require.Equal(t, now, record.UpdatedAt)
	require.Equal(t, now.Add(time.Second), record.ExpiresAt)
}

func TestRecordFromControlEventUpdateWithoutTTLDoesNotExpire(t *testing.T) {
	now := time.Unix(200, 0)

	record, consumed, err := recordFromEventAt("container-a", ControlEventTitle, ControlEventSourceType, "heartbeat", []string{"gpu_job_id:job-a"}, DefaultTTL, now)
	require.NoError(t, err)
	require.True(t, consumed)
	require.True(t, record.ExpiresAt.IsZero())
}

func TestRecordFromControlEventEnd(t *testing.T) {
	now := time.Unix(300, 0)

	record, consumed, err := recordFromEventAt("container-a", ControlEventTitle, ControlEventSourceType, "end", nil, time.Minute, now)
	require.NoError(t, err)
	require.True(t, consumed)
	require.Equal(t, "container-a", record.ContainerID)
	require.Equal(t, ActionEnd, record.Action)
	require.Empty(t, record.JobID)
	require.Empty(t, record.Tags)
	require.Equal(t, now, record.UpdatedAt)
	require.True(t, record.ExpiresAt.IsZero())
}

func TestRecordFromControlEventIgnoresRegularEvents(t *testing.T) {
	record, consumed, err := recordFromEventAt("container-a", "regular event", ControlEventSourceType, "start", []string{"gpu_job_id:job-a"}, time.Minute, time.Now())
	require.NoError(t, err)
	require.False(t, consumed)
	require.Empty(t, record)
}

func TestRecordFromControlEventValidation(t *testing.T) {
	_, consumed, err := recordFromEventAt("", ControlEventTitle, ControlEventSourceType, "start", []string{"gpu_job_id:job-a"}, time.Minute, time.Now())
	require.True(t, consumed)
	require.True(t, errors.Is(err, ErrNoContainerID))

	_, consumed, err = recordFromEventAt("container-a", ControlEventTitle, ControlEventSourceType, "start", []string{"team:ml"}, time.Minute, time.Now())
	require.True(t, consumed)
	require.True(t, errors.Is(err, ErrNoJobID))

	_, consumed, err = recordFromEventAt("container-a", ControlEventTitle, ControlEventSourceType, "unknown", []string{"gpu_job_id:job-a"}, time.Minute, time.Now())
	require.True(t, consumed)
	require.True(t, errors.Is(err, ErrUnsupportedAction))
}
