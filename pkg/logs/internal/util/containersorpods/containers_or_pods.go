// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containersorpods

import (
	"context"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
)

// LogWhat is the answer this package provides
type LogWhat int

const (
	// LogContainers means that the logs-agent should log containers, not pods.
	LogContainers LogWhat = iota

	// LogPods means that the logs-agent should log pods, not containers.
	LogPods

	// LogUnknown indicates that it's too early to tell which should be used, because
	// neither service has become available yet.
	LogUnknown

	// LogNothing means neither containers nor pods are supported.
	LogNothing
)

func (lw LogWhat) String() string {
	switch lw {
	case LogContainers:
		return "LogContainers"
	case LogPods:
		return "LogPods"
	case LogNothing:
		return "LogNothing"
	default:
		return "LogUnknown"
	}
}

// Chooser determines how the logs-agent should handle containers:
// either monitoring individual containers, or monitoring pods and logging the
// containers within them.
//
// The decision is rather complex:
//
//     - if any of the container features (docker, containerd, cri, podman) are
//       present and kubernetes is not, wait for the dockerutil service to start and
//       return LogContainers
//     - if the kubernetes feature is available and no container features are
//       available, wait for the kubelet service to start, and return LogPods
//     - if none of the features are available, LogNothing
//     - if at least one container feature _and_ the kubernetes feature are available,
//       then wait for either of the dockerutil service or the kubelet service to start.
//       This always tries both at the same time, and if both are available will
//       return LogPods if `logs_config.k8s_container_use_file` is true or
//       LogContainers if the configuration setting is false.
//
// If this function returns LogPods, then the caller may assume the kubelet
// service is available. Similarly, if this function returns LogContainers,
// then the caller may assume the dockerutil service is available.
//
// The dependency on the configuration value is based on the observation that
// the kubernetes launcher always uses files to log pods, while the docker
// launcher (in some circumstances, at least) does not.
type Chooser interface {
	// Wait blocks until a decision is made, or the given context is cancelled.
	Wait(ctx context.Context) LogWhat

	// Get is identical to Wait, except that it returns LogUnknown immediately
	// in any situation where Wait would wait.
	Get() LogWhat
}

type chooser struct {
	// choice carries the result, once it is known.
	choice chan LogWhat

	// m covers the 'started' field
	m sync.Mutex

	// if started is true, then the process of choosing has begun
	// and it is safe to wait for a value in `choice`.
	started bool

	// kubeletReady determines if kubelet is ready, or how long to wait
	kubeletReady func() (bool, time.Duration)

	// dockerReady determines if dockerutil is ready, or how long to wait
	dockerReady func() (bool, time.Duration)
}

// NewChooser returns a new Chooser.
func NewChooser() Chooser {
	return &chooser{
		choice:       make(chan LogWhat, 1),
		kubeletReady: kubernetesReady,
		dockerReady:  dockerReady,
	}
}

// NewDecidedChooser returns a new Chooser where the choice is predetermined.
// This is for use in unit tests.
func NewDecidedChooser(decision LogWhat) Chooser {
	ch := &chooser{
		choice:       make(chan LogWhat, 1),
		kubeletReady: kubernetesReady,
		dockerReady:  dockerReady,
	}
	ch.started = true
	ch.choice <- decision
	return ch
}

// Wait implements Chooser#Wait.
func (ch *chooser) Wait(ctx context.Context) LogWhat {
	ch.start()
	select {
	case c := <-ch.choice:
		// put the value back in the channel for the next querier
		ch.choice <- c
		return c
	case <-ctx.Done():
		return LogUnknown
	}
}

// Get implements Chooser#Get.
func (ch *chooser) Get() LogWhat {
	ch.start()
	select {
	case c := <-ch.choice:
		// put the value back in the channel for the next querier
		ch.choice <- c
		return c
	default:
		return LogUnknown
	}
}

func min(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

func (ch *chooser) start() {
	ch.m.Lock()
	if !ch.started {
		go ch.choose(true)
		ch.started = true
	}
	ch.m.Unlock()
}

// preferred returns the preferred LogWhat, based on configuration
func (ch *chooser) preferred() LogWhat {
	if config.Datadog.GetBool("logs_config.k8s_container_use_file") {
		return LogPods
	}
	return LogContainers
}

// choose runs in a dedicated goroutine and makes a choice between LogPods and
// LogContainers, sending the result to ch.choice.  If wait is true, then if no
// choice is available yet, it will wait until a choice becomes available.
func (ch *chooser) choose(wait bool) {
	c := config.IsFeaturePresent(config.Docker) ||
		config.IsFeaturePresent(config.Containerd) ||
		config.IsFeaturePresent(config.Cri) ||
		config.IsFeaturePresent(config.Podman)
	k := config.IsFeaturePresent(config.Kubernetes)

	for {
		var delay time.Duration
		var ready bool

		switch {
		case c && !k:
			ready, delay = ch.dockerReady()
			if ready {
				ch.choice <- LogContainers
				return
			}
			// otherwise, wait

		case k && !c:
			ready, delay = ch.kubeletReady()
			if ready {
				ch.choice <- LogPods
				return
			}
			// otherwise, wait

		case k && c:
			dready, ddelay := ch.dockerReady()
			kready, kdelay := ch.kubeletReady()
			switch {
			case dready && !kready:
				ch.choice <- LogContainers
				return
			case kready && !dready:
				ch.choice <- LogPods
				return
			case kready && dready:
				ch.choice <- ch.preferred()
				return
			default:
				// otherwise, wait
				delay = min(ddelay, kdelay)
			}

		default:
			ch.choice <- LogNothing
			return
		}

		if wait {
			time.Sleep(delay)
		} else {
			return
		}
	}
}
