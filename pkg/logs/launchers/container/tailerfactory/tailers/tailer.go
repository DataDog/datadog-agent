// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tailers

import (
	"context"
	"time"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	containerTailerPkg "github.com/DataDog/datadog-agent/pkg/logs/tailers/container"
	"github.com/DataDog/datadog-agent/pkg/util/backoff"
	dockerutilPkg "github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	backoffMaxSeconds = 60
	backoffMaxRetries = 10
)

// Tailer is the interface implemented by both DockerSocketTailer and APITailer
type Tailer interface {
	Start() error
	Stop()
	GetContainerID() string
	GetContext() context.Context
	close()
}

// Base wraps pkg/logs/tailers/container.Tailer to satisfy
// the container launcher's `Tailer` interface, and to handle the
// erroredContainerID channel.
//
// NOTE: once the docker launcher is removed, the inner Docker tailer can be
// modified to suit the Tailer interface directly and to handle connection
// failures on its own, and this wrapper will no longer be necessary.
type base struct {
	ContainerID string
	source      *sources.LogSource
	pipeline    chan *message.Message
	readTimeout time.Duration
	tagger      tagger.Component

	// registry is used to calculate `since`
	registry auditor.Registry

	// ctx controls the run loop
	ctx context.Context

	// cancel stops the run loop
	cancel context.CancelFunc

	// stopped is closed when the run loop finishes
	stopped chan struct{}
}

// Stop implements Tailer#Stop.
func (t *base) Stop() {
	t.cancel()
	t.cancel = nil
	<-t.stopped
}

// stopTailer stops the inner tailer.
func (t *base) stopTailer(inner *containerTailerPkg.Tailer) {
	inner.Stop()
}

// Close implements Tailer#close
func (t *base) close() {
	close(t.stopped)
}

// GetContainerID implements Tailer#GetContainerID
func (t *base) GetContainerID() string {
	return t.ContainerID
}

// GetContext implements Tailer#GetContext
func (t *base) GetContext() context.Context {
	return t.ctx
}

// run implements a loop to monitor the tailer and re-create it if it fails.  It takes
// pointers to tryStartTailer and stopTailer to support testing.
func (t *base) run(
	tryStartTailer func() (*containerTailerPkg.Tailer, chan string, error),
	stopTailer func(*containerTailerPkg.Tailer),
) {
	defer t.close()

	numErrors := 0

	policy := backoff.NewExpBackoffPolicy(2, 1, 60, backoffMaxSeconds, false)

	for {
		var backoffTimerC <-chan time.Time
		inner, erroredContainerID, err := tryStartTailer()

		if err != nil {
			numErrors = policy.IncError(numErrors)

			if numErrors > backoffMaxRetries {
				log.Warnf("Could not tail container %v: %v",
					dockerutilPkg.ShortContainerID(t.GetContainerID()), err)
				return
			}

			duration := policy.GetBackoffDuration(numErrors)
			backoffTimerC = time.After(duration)

		} else {
			// reset the number of errors
			numErrors = 0
		}

		select {
		case <-t.GetContext().Done():
			// the launcher has requested that the tailer stop
			if inner != nil {
				// Ensure any pending errors are cleared when we try to stop the tailer. Since erroredContainerID
				// is unbuffered, any pending writes to this channel could cause a deadlock as the tailers stop
				// condition is managed in the same goroutine in containerTailerPkg.
				go func() {
					//nolint:revive // TODO(AML) Fix revive linter
					for range erroredContainerID {
					}
				}()
				stopTailer(inner)
				if erroredContainerID != nil {
					close(erroredContainerID)
				}
			}
			return

		case <-erroredContainerID:
			// the inner tailer has failed after it has started
			if inner != nil {
				stopTailer(inner)
			}
			continue // retry

		case <-backoffTimerC:
			// it's time to retry starting the tailer
			continue
		}
	}
}
