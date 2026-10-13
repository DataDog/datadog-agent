// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package gpu

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/healthplatform/issues/gpuenvironment"
	healthplatformmock "github.com/DataDog/datadog-agent/comp/healthplatform/store/mock"
	"github.com/DataDog/datadog-agent/pkg/gpu/containers"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
)

func TestSysprobeDataTracker(t *testing.T) {
	tracker := newSysprobeDataTracker()
	start := time.Now()
	errUnreachable := &net.OpError{Op: "dial", Net: "unix", Err: syscall.ENOENT}

	tracker.record(gpuenvironment.SystemProbeSourceEBPF, errUnreachable, start)
	tracker.record(gpuenvironment.SystemProbeSourcePRM, nil, start)
	assert.Empty(t, tracker.failures(start.Add(sysprobeDataUnavailableThreshold-time.Second)))

	// The first failure time is kept across consecutive failures.
	tracker.record(gpuenvironment.SystemProbeSourceEBPF, errUnreachable, start.Add(time.Minute))
	failures := tracker.failures(start.Add(sysprobeDataUnavailableThreshold))
	require.Len(t, failures, 1)
	assert.Equal(t, gpuenvironment.SystemProbeSourceEBPF, failures[0].Source)
	assert.Equal(t, gpuenvironment.SystemProbeErrorUnreachable, failures[0].ErrorClass)
	assert.Equal(t, errUnreachable.Error(), failures[0].Error)

	// A success resets the source.
	tracker.record(gpuenvironment.SystemProbeSourceEBPF, nil, start.Add(2*time.Minute))
	tracker.record(gpuenvironment.SystemProbeSourceEBPF, errUnreachable, start.Add(3*time.Minute))
	assert.Empty(t, tracker.failures(start.Add(sysprobeDataUnavailableThreshold+time.Minute)))
}

func TestClassifySysprobeError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{"module not loaded", fmt.Errorf("refresh: %w", &sysprobeclient.StatusCodeError{StatusCode: http.StatusNotFound}), gpuenvironment.SystemProbeErrorModuleNotLoaded},
		{"source disabled", &sysprobeclient.StatusCodeError{StatusCode: http.StatusServiceUnavailable}, gpuenvironment.SystemProbeErrorSourceDisabled},
		{"server error", &sysprobeclient.StatusCodeError{StatusCode: http.StatusInternalServerError}, gpuenvironment.SystemProbeErrorOther},
		{"deadline exceeded", fmt.Errorf("get: %w", context.DeadlineExceeded), gpuenvironment.SystemProbeErrorTimeout},
		{"socket missing", &net.OpError{Op: "dial", Net: "unix", Err: syscall.ENOENT}, gpuenvironment.SystemProbeErrorUnreachable},
		{"connection refused", fmt.Errorf("dial: %w", syscall.ECONNREFUSED), gpuenvironment.SystemProbeErrorUnreachable},
		{"other", errors.New("invalid JSON"), gpuenvironment.SystemProbeErrorOther},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, classifySysprobeError(tt.err))
		})
	}
}

func TestSyncSysprobeDataHealthIssue(t *testing.T) {
	healthStore := healthplatformmock.New(t)
	check := &Check{sysprobeData: newSysprobeDataTracker()}
	check.SetIssueReporter(healthStore)
	start := time.Now()

	check.sysprobeData.record(gpuenvironment.SystemProbeSourceDriverEvents, &sysprobeclient.StatusCodeError{StatusCode: http.StatusNotFound}, start)
	check.syncSysprobeDataHealthIssue(start)
	assert.Nil(t, healthStore.GetIssue(gpuenvironment.SystemProbeDataIssueID))

	check.syncSysprobeDataHealthIssue(start.Add(sysprobeDataUnavailableThreshold))
	issue := healthStore.GetIssue(gpuenvironment.SystemProbeDataIssueID)
	require.NotNil(t, issue)
	assert.Equal(t, gpuenvironment.SystemProbeDataIssueName, issue.IssueName)
	assert.Equal(t, gpuenvironment.SystemProbeDataIssueID, issue.Id)
	assert.Contains(t, issue.Title, gpuenvironment.SystemProbeSourceDriverEvents)

	check.sysprobeData.record(gpuenvironment.SystemProbeSourceDriverEvents, nil, start.Add(sysprobeDataUnavailableThreshold+time.Minute))
	check.syncSysprobeDataHealthIssue(start.Add(sysprobeDataUnavailableThreshold + time.Minute))
	assert.Nil(t, healthStore.GetIssue(gpuenvironment.SystemProbeDataIssueID))
}

func TestSyncDRACDIHealthIssue(t *testing.T) {
	original := containers.CDISpecDirs
	t.Cleanup(func() { containers.CDISpecDirs = original })
	specDir := t.TempDir()
	containers.CDISpecDirs = []string{specDir}

	healthStore := healthplatformmock.New(t)
	check := &Check{}
	check.SetIssueReporter(healthStore)
	start := time.Now()

	// No unresolved claims: a directory without specs is not an issue.
	check.syncDRACDIHealthIssue(0, start.Add(draCDISpecsMissingThreshold))
	assert.Nil(t, healthStore.GetIssue(gpuenvironment.DRACDIIssueID))

	// Below the threshold nothing is reported.
	check.syncDRACDIHealthIssue(2, start)
	check.syncDRACDIHealthIssue(2, start.Add(draCDISpecsMissingThreshold-time.Second))
	assert.Nil(t, healthStore.GetIssue(gpuenvironment.DRACDIIssueID))

	check.syncDRACDIHealthIssue(2, start.Add(draCDISpecsMissingThreshold))
	issue := healthStore.GetIssue(gpuenvironment.DRACDIIssueID)
	require.NotNil(t, issue)
	assert.Equal(t, gpuenvironment.DRACDIIssueName, issue.IssueName)
	assert.Equal(t, gpuenvironment.DRACDIIssueID, issue.Id)
	assert.Contains(t, issue.Title, specDir)
	assert.Equal(t, "2", issue.GetExtra().GetFields()[gpuenvironment.DRACDIContextKeyContainerCount].GetStringValue())

	// NVIDIA DRA claim specs appearing resolve the issue, even with unresolved claims.
	require.NoError(t, os.WriteFile(filepath.Join(specDir, containers.NvidiaDRAClaimSpecFileName("other")), []byte("{}"), 0o644))
	check.syncDRACDIHealthIssue(2, start.Add(2*draCDISpecsMissingThreshold))
	assert.Nil(t, healthStore.GetIssue(gpuenvironment.DRACDIIssueID))
}

func TestHealthIssueSyncWithNilReporter(t *testing.T) {
	check := &Check{sysprobeData: newSysprobeDataTracker()}

	require.NotPanics(t, func() {
		check.syncSysprobeDataHealthIssue(time.Now())
		check.syncDRACDIHealthIssue(1, time.Now())
	})
}
