// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dockerork8s

import (
	"context"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/launchers/docker"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/launchers/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
)

// Choice represents a choice made by DockerOrK8s.
type Choice int

const (
	pendingChoice Choice = iota

	// Docker indicates that the component has chosen Docker.
	Docker

	// K8s indicates that the component has chosen K8s.
	K8s

	// None indicates that the component has determined that neither Docker nor
	// Kubernetes will be chosen.
	None
)

// DockerOrK8s implements the logs agent's distinction between running in
// Docker and running in Kubernetes.
//
// It must make a choice between one or the other, as running both would entail
// logging each container twice.  A complicating factor is that either daemon
// may not be running yet when the agent starts.
type DockerOrK8s struct {
	// m protects the fields 'choice' and 'subscribers'
	m sync.Mutex

	// choice is the choice made by this value
	choice Choice

	// channels waiting to hear about a choice.  These channels will receive a single
	// value when a choice is made, and then be closed.
	subscribers []chan Choice

	// stop will stop ongoing attempts to make a choice
	stop context.CancelFunc

	// stopped is closed when the `run` goroutine completes
	stopped chan struct{}

	// preferred is the preferred choice, if both are available
	preferred Choice

	// isAvailable contains non-blocking functions to detect when each choice
	// can be made, by returning true.  If the choice cannot be made, then they
	// return a retrier indicating when to try again.
	isAvailable map[Choice]func() (bool, *retry.Retrier)
}

// New creates a new DockerOrK8s.
func New(preferred Choice) *DockerOrK8s {
	return &DockerOrK8s{
		choice:    pendingChoice,
		stopped:   make(chan struct{}),
		preferred: preferred,
		isAvailable: map[Choice]func() (bool, *retry.Retrier){
			Docker: docker.IsAvailable,
			K8s:    kubernetes.IsAvailable,
		},
	}
}

// Start begins the decision-making process, asynchronously.
func (dk *DockerOrK8s) Start() {
	dk.m.Lock()
	defer dk.m.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	dk.stop = cancel
	go dk.run(ctx)
}

// Stop stops attempts to make a choice, and waits until the goroutine has finished.
func (dk *DockerOrK8s) Stop() {
	dk.stop()
	<-dk.stopped

	// inform any remaining subscribers that no choice is forthcoming
	dk.m.Lock()
	defer dk.m.Unlock()
	dk.choice = None
	dk.notify()
}

// Subscribe subscribes to get a notification when a choice is made, by receiving a
// Choice on the returned channel.  If a choice has already been made, the value will
// be returned immediately.  If no choice is made before the component stops, then
// the channel will receive NoneChosen.
func (dk *DockerOrK8s) Subscribe() <-chan Choice {
	dk.m.Lock()
	defer dk.m.Unlock()
	ch := make(chan Choice, 1)
	dk.subscribers = append(dk.subscribers, ch)
	dk.notify()
	return ch
}

// run implements the search for a choice.
func (dk *DockerOrK8s) run(ctx context.Context) {
	defer close(dk.stopped)

	for {
		choice, nextRetry := dk.attempt(ctx)
		if choice != pendingChoice {
			dk.m.Lock()
			dk.choice = choice
			dk.notify()
			dk.m.Unlock()
			break
		}

		select {
		case <-time.After(nextRetry):
		case <-ctx.Done():
			return
		}
	}
}

// attempt attempts to make a choice, calling the functions in isAvailable.
func (dk *DockerOrK8s) attempt(ctx context.Context) (Choice, time.Duration) {
	// try the preferred choice first
	choice := dk.preferred
	ok, rt1 := dk.isAvailable[choice]()
	if ok {
		return choice, 0
	}

	// try the opposite choice second
	choice = choice.opposite()
	ok, rt2 := dk.isAvailable[choice]()
	if ok {
		return choice, 0
	}

	// Hold on to the retrier with the longest interval
	if rt1 == nil || (rt2 != nil && rt1.NextRetry().Before(rt2.NextRetry())) {
		rt1 = rt2
	}

	if rt1 == nil {
		return None, 0
	}

	nextRetry := time.Until(rt1.NextRetry())
	return pendingChoice, nextRetry
}

// notify notifies any subscribers if a choice has been made.  This must be called with
// dk.m locked.
func (dk *DockerOrK8s) notify() {
	if dk.choice == pendingChoice {
		return
	}

	for _, ch := range dk.subscribers {
		ch <- dk.choice
		close(ch)
	}
	dk.subscribers = nil
}

// opposite returns the opposite choice
func (c Choice) opposite() Choice {
	switch c {
	case Docker:
		return K8s
	case K8s:
		return Docker
	default:
		return c
	}
}
