// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

//nolint:revive // TODO(AML) Fix revive linter
package tailers

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	apiTailerPkg "github.com/DataDog/datadog-agent/pkg/logs/tailers/api"
	dockerutilPkg "github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
)

// ApiTailer wraps pkg/logs/tailers/docker.Tailer to satisfy
// the container launcher's `Tailer` interface, and to handle the
// erroredContainerID channel.
//
// NOTE: once the docker launcher is removed, the inner Docker tailer can be
// modified to suit the Tailer interface directly and to handle connection
// failures on its own, and this wrapper will no longer be necessary.
type ApiTailer struct {
	// arguments to dockerTailerPkg.NewTailer (except erroredContainerID)

	kubeUtil      kubelet.KubeUtilInterface
	ContainerID   string
	ContainerName string
	PodName       string
	PodNamespace  string
	source        *sources.LogSource
	pipeline      chan *message.Message
	readTimeout   time.Duration
	tagger        tagger.Component

	// registry is used to calculate `since`
	registry auditor.Registry

	// ctx controls the run loop
	ctx context.Context

	// cancel stops the run loop
	cancel context.CancelFunc

	// stopped is closed when the run loop finishes
	stopped chan struct{}
}

// NewApiTailer Creates a new docker socket tailer
func NewApiTailer(kubeutil kubelet.KubeUtilInterface, containerID, containerName, podName, podNamespace string, source *sources.LogSource, pipeline chan *message.Message, readTimeout time.Duration, registry auditor.Registry, tagger tagger.Component) *ApiTailer {
	return &ApiTailer{
		kubeUtil:      kubeutil,
		ContainerID:   containerID,
		ContainerName: containerName,
		PodName:       podName,
		PodNamespace:  podNamespace,
		source:        source,
		pipeline:      pipeline,
		readTimeout:   readTimeout,
		registry:      registry,
		tagger:        tagger,
		ctx:           nil,
		cancel:        nil,
		stopped:       nil,
	}
}

// tryStartTailer tries to start the inner tailer, returning an erroredContainerID channel if
// successful.
func (t *ApiTailer) tryStartTailer() (*apiTailerPkg.Tailer, chan string, error) {
	erroredContainerID := make(chan string)
	inner := apiTailerPkg.NewTailer(
		t.kubeUtil,
		t.ContainerID,
		t.ContainerName,
		t.PodName,
		t.PodNamespace,
		t.source,
		t.pipeline,
		erroredContainerID,
		t.readTimeout,
		t.tagger,
	)
	since, err := since(t.registry, inner.Identifier())
	if err != nil {
		log.Warnf("Could not recover tailing from last committed offset %v: %v",
			dockerutilPkg.ShortContainerID(t.ContainerID), err)
		// (the `since` value is still valid)
	}

	err = inner.Start(since)
	if err != nil {
		return nil, nil, err
	}
	return inner, erroredContainerID, nil
}

// stopTailer stops the inner tailer.
func (t *ApiTailer) stopTailer(inner *apiTailerPkg.Tailer) {
	inner.Stop()
}

// Start implements Tailer#Start.
func (t *ApiTailer) Start() error {
	t.ctx, t.cancel = context.WithCancel(context.Background())
	t.stopped = make(chan struct{})
	go t.run(t.tryStartTailer, t.stopTailer)
	return nil
}

// Stop implements Tailer#Stop.
func (t *ApiTailer) Stop() {
	t.cancel()
	t.cancel = nil
	<-t.stopped
}

// run implements a loop to monitor the tailer and re-create it if it fails.  It takes
// pointers to tryStartTailer and stopTailer to support testing.
func (t *ApiTailer) run(
	tryStartTailer func() (*apiTailerPkg.Tailer, chan string, error),
	stopTailer func(*apiTailerPkg.Tailer),
) {
	defer close(t.stopped)

	backoffDuration := backoffInitialDuration

	for {
		var backoffTimerC <-chan time.Time

		// try to start the inner tailer
		inner, erroredContainerID, err := tryStartTailer()
		if err != nil {
			if backoffDuration > backoffMaxDuration {
				log.Warnf("Could not tail container %v: %v",
					dockerutilPkg.ShortContainerID(t.ContainerID), err)
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
		case <-t.ctx.Done():
			// the launcher has requested that the tailer stop
			if inner != nil {
				// Ensure any pending errors are cleared when we try to stop the tailer. Since erroredContainerID
				// is unbuffered, any pending writes to this channel could cause a deadlock as the tailers stop
				// condition is managed in the same goroutine in dockerTailerPkg.
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
