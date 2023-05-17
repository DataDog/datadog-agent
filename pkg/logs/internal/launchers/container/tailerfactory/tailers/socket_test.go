// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package tailers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

	dockerTailerPkg "github.com/DataDog/datadog-agent/pkg/logs/internal/tailers/docker"
)

func TestDockerSocketTailer_run_normal_stop(t *testing.T) {
	dst := &DockerSocketTailer{}
	dst.ctx, dst.cancel = context.WithCancel(context.Background())
	dst.stopped = make(chan struct{})

	tailerStarted := false
	tailerStopped := false

	// emulate dst.Start(), but with fake tryStartTailer and stopTailer
	go dst.run(
		func() (*dockerTailerPkg.Tailer, chan string, error) {
			tailerStarted = true
			// fake a successful tailer start
			return &dockerTailerPkg.Tailer{}, nil, nil
		},
		func(*dockerTailerPkg.Tailer) {
			tailerStopped = true
		})

	dst.Stop()

	// check that the tailer was started and subsequently stopped
	require.True(t, tailerStarted)
	require.True(t, tailerStopped)
}

func TestDockerSocketTailer_run_erroredContainer(t *testing.T) {
	dst := &DockerSocketTailer{}
	dst.ctx, dst.cancel = context.WithCancel(context.Background())
	dst.stopped = make(chan struct{})

	tailerStarted := atomic.NewInt32(0)
	tailerStopped := atomic.NewInt32(0)

	// emulate dst.Start(), but with fake tryStartTailer and stopTailer
	go dst.run(
		func() (*dockerTailerPkg.Tailer, chan string, error) {
			erroredContainerID := make(chan string)
			if tailerStarted.Inc() < 3 {
				// have the tailer fail after starting successfully
				go func() {
					time.Sleep(10 * time.Millisecond)
					erroredContainerID <- "abcd"
				}()
			}
			return &dockerTailerPkg.Tailer{}, erroredContainerID, nil
		},
		func(*dockerTailerPkg.Tailer) {
			tailerStopped.Inc()
		})

	// wait until the inner tailer has started a third time due to two errors
	for tailerStarted.Load() < 3 {
		time.Sleep(1 * time.Millisecond)
	}

	// stop the tailer
	dst.Stop()

	// check that the tailer was started and subsequently stopped twice
	require.Equal(t, int32(3), tailerStarted.Load())
	require.Equal(t, int32(3), tailerStopped.Load())
}

func TestDockerSocketTailer_run_canStopWithError(t *testing.T) {
	dst := &DockerSocketTailer{}
	dst.ctx, dst.cancel = context.WithCancel(context.Background())
	dst.stopped = make(chan struct{})

	tailerStarted := atomic.NewInt32(0)
	tailerStopped := atomic.NewInt32(0)

	// emulate dst.Start(), but with fake tryStartTailer and stopTailer
	erroredContainerID := make(chan string)
	go dst.run(
		func() (*dockerTailerPkg.Tailer, chan string, error) {
			tailerStarted.Inc()
			return &dockerTailerPkg.Tailer{}, erroredContainerID, nil
		},
		func(*dockerTailerPkg.Tailer) {
			// Simulate an error occurring at the same time as as the tailer is trying to stop.
			// This can happen in the real socket tailer implementation as these errors are handled by
			// the same goroutine that manages the tailer shutdown. This test ensures any pending errors
			// do not prevent the tailer from being stopped.
			erroredContainerID <- "abcd"
			tailerStopped.Inc()
		})

	// wait until the inner tailer has started
	for tailerStarted.Load() < 1 {
		time.Sleep(1 * time.Millisecond)
	}

	// stop the tailer - this should not block.
	dst.Stop()

	// check that the tailer was started and subsequently stopped
	require.Equal(t, int32(1), tailerStarted.Load())
	require.Equal(t, int32(1), tailerStopped.Load())
}

func TestDockerSocketTailer_run_error_starting(t *testing.T) {
	backoffInitialDuration = 1 * time.Millisecond
	defer func() { backoffInitialDuration = 1 * time.Second }()

	dst := &DockerSocketTailer{}
	dst.ctx, dst.cancel = context.WithCancel(context.Background())
	dst.stopped = make(chan struct{})

	tailerStarted := atomic.NewInt32(0)
	tailerStopped := atomic.NewInt32(0)

	// emulate dst.Start(), but with fake tryStartTailer and stopTailer
	go dst.run(
		func() (*dockerTailerPkg.Tailer, chan string, error) {
			if tailerStarted.Inc() < 3 {
				return nil, nil, errors.New("uhoh")
			}
			return &dockerTailerPkg.Tailer{}, nil, nil
		},
		func(*dockerTailerPkg.Tailer) {
			tailerStopped.Inc()
		})

	// wait until the inner tailer has started a third time due to two errors
	for tailerStarted.Load() < 3 {
		time.Sleep(1 * time.Millisecond)
	}

	// stop the tailer
	dst.Stop()

	// check that the tailer was started three times (successful the third time)
	require.Equal(t, int32(3), tailerStarted.Load())
	// .. and subsequently stopped once
	require.Equal(t, int32(1), tailerStopped.Load())
}

func TestDockerSocketTailer_run_error_starting_expires(t *testing.T) {
	backoffInitialDuration = 1 * time.Millisecond
	backoffMaxDuration = 10 * time.Millisecond
	defer func() {
		backoffInitialDuration = 1 * time.Second
		backoffMaxDuration = 60 * time.Second
	}()

	dst := &DockerSocketTailer{}
	dst.ctx, dst.cancel = context.WithCancel(context.Background())
	dst.stopped = make(chan struct{})

	tailerStarted := atomic.NewInt32(0)
	tailerStopped := atomic.NewInt32(0)

	// emulate dst.Start(), but with fake tryStartTailer and stopTailer
	go dst.run(
		func() (*dockerTailerPkg.Tailer, chan string, error) {
			tailerStarted.Inc()
			return nil, nil, errors.New("uhoh")
		},
		func(*dockerTailerPkg.Tailer) {
			tailerStopped.Inc()
		})

	// wait until the tailer stops itself after giving up
	<-dst.stopped

	// check that the tailer was started five times (with delays of 1 + 2 + 4 + 8ms between)
	require.Equal(t, int32(5), tailerStarted.Load())
	// .. and never succeeded, so never started
	require.Equal(t, int32(0), tailerStopped.Load())
}
