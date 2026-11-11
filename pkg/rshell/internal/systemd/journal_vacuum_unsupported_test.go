// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !linux && !darwin

package systemd

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

func TestVacuumJournalFailsClosedOffUnix(t *testing.T) {
	now := time.Now()
	result, err := NewClient(Target{}).VacuumJournal(context.Background(), builtins.JournalVacuumRequest{
		Now:    now,
		Before: now.Add(-48 * time.Hour),
	})
	assert.Zero(t, result.Files)
	assert.Zero(t, result.Bytes)
	require.ErrorIs(t, err, builtins.ErrSystemdUnsupported)
}
