// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package gpu

import (
	"context"
	"errors"
	"net"
	"net/http"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/comp/healthplatform/issues/gpuenvironment"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
)

// sysprobeDataUnavailableThreshold is how long a system-probe data source must
// keep failing before it is reported, so that transient failures (system-probe
// restarts, slow startup) don't create issues.
const sysprobeDataUnavailableThreshold = 5 * time.Minute

type sysprobeSourceState struct {
	firstFailure time.Time
	lastErr      error
}

// sysprobeDataTracker tracks, for each system-probe data source the check
// expects data from, since when it has been failing. Not thread-safe: it is
// only used from the Run goroutine.
type sysprobeDataTracker struct {
	sources map[string]*sysprobeSourceState
}

func newSysprobeDataTracker() *sysprobeDataTracker {
	return &sysprobeDataTracker{sources: make(map[string]*sysprobeSourceState)}
}

// record stores the result of the latest refresh of a source.
func (t *sysprobeDataTracker) record(source string, err error, now time.Time) {
	state, ok := t.sources[source]
	if !ok {
		state = &sysprobeSourceState{}
		t.sources[source] = state
	}

	if err == nil {
		state.firstFailure = time.Time{}
		state.lastErr = nil
		return
	}

	if state.firstFailure.IsZero() {
		state.firstFailure = now
	}
	state.lastErr = err
}

// failures returns the sources that have been failing for at least sysprobeDataUnavailableThreshold.
func (t *sysprobeDataTracker) failures(now time.Time) []gpuenvironment.SystemProbeSourceFailure {
	var failures []gpuenvironment.SystemProbeSourceFailure
	for source, state := range t.sources {
		if state.lastErr == nil || now.Sub(state.firstFailure) < sysprobeDataUnavailableThreshold {
			continue
		}
		failures = append(failures, gpuenvironment.SystemProbeSourceFailure{
			Source:     source,
			ErrorClass: classifySysprobeError(state.lastErr),
			Error:      state.lastErr.Error(),
		})
	}
	slices.SortFunc(failures, func(a, b gpuenvironment.SystemProbeSourceFailure) int {
		return strings.Compare(a.Source, b.Source)
	})
	return failures
}

func classifySysprobeError(err error) string {
	var statusErr *sysprobeclient.StatusCodeError
	if errors.As(err, &statusErr) {
		switch statusErr.StatusCode {
		case http.StatusNotFound:
			return gpuenvironment.SystemProbeErrorModuleNotLoaded
		case http.StatusNotImplemented:
			return gpuenvironment.SystemProbeErrorSourceDisabled
		case http.StatusServiceUnavailable:
			return gpuenvironment.SystemProbeErrorSourceUnavailable
		}
	}

	var netErr net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
		return gpuenvironment.SystemProbeErrorTimeout
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) || errors.Is(err, syscall.ENOENT) || errors.Is(err, syscall.ECONNREFUSED) {
		return gpuenvironment.SystemProbeErrorUnreachable
	}

	return gpuenvironment.SystemProbeErrorOther
}
