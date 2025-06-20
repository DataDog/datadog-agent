// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet || docker

package tailers

import (
	"context"
	"time"

	containerutilPkg "github.com/DataDog/datadog-agent/pkg/util/containers"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	auditor "github.com/DataDog/datadog-agent/comp/logs/auditor/def"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	containerTailerPkg "github.com/DataDog/datadog-agent/pkg/logs/tailers/container"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	backoffInitialDuration = 1 * time.Second
	backoffMaxDuration     = 60 * time.Second
)

// Tailer is the interface implemented by both DockerSocketTailer and APITailer
type Tailer interface {
	Start() error
	Stop()
	GetContainerID() string
	GetContext() context.Context
}

// Base wraps pkg/logs/tailers/container.Tailer to satisfy
// the container launcher's `Tailer` interface, and to handle the
// erroredContainerID channel.
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

	backoffDuration := backoffInitialDuration

	for {
		var backoffTimerC <-chan time.Time

		// try to start the inner tailer
		inner, erroredContainerID, err := tryStartTailer()
		if err != nil {
			if backoffDuration > backoffMaxDuration {
				log.Warnf("Could not tail container %v: %v",
					containerutilPkg.ShortContainerID(t.ContainerID), err)
				return
			}
			// set up to wait before trying again
			backoffTimerC = time.After(backoffDuration)
			backoffDuration *= 2
		} else {
			// success, so reset backoff
			backoffTimerC = nil
			backoffDuration = backoffInitialDuration
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
