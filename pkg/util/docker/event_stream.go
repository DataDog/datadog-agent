// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build docker

package docker

import (
	"context"
	"io"
	"strconv"
	"time"

	log "github.com/cihub/seelog"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"

	"github.com/DataDog/datadog-agent/pkg/status/health"
)

//// eventStreamState logic unit tested in event_stream_test.go
//// DockerUtil logic covered by the listeners/docker and dogstatsd/origin_detection integration tests.

const eventSendTimeout = 100 * time.Millisecond
const eventSendBuffer = 5

// SubscribeToContainerEvents allows a package to subscribe to events from the event stream.
// A unique subscriber name should be provided.
//
// Attention: events sent through the channel are references to objects shared between subscribers,
// subscribers should NOT modify them.
func (d *DockerUtil) SubscribeToContainerEvents(name string) (<-chan *ContainerEvent, <-chan error, error) {
	eventChan, errorChan, err, shouldStart := d.eventState.subscribe(name)

	if shouldStart {
		d.eventState.Lock()
		d.eventState.running = true
		go d.dispatchEvents(d.eventState.cancelChan)
		d.eventState.Unlock()
	}

	return eventChan, errorChan, err
}

// extracted from SubscribeToContainerEvents for unit testing, additional boolean
// indicates whether the dispatch goroutine should be started by DockerUtil
func (e *eventStreamState) subscribe(name string) (<-chan *ContainerEvent, <-chan error, error, bool) {
	var shouldStart bool
	e.RLock()
	if _, found := e.subscribers[name]; found {
		e.RUnlock()
		return nil, nil, ErrAlreadySubscribed, false
	}
	e.RUnlock()

	sub := &eventSubscriber{
		name:      name,
		eventChan: make(chan *ContainerEvent, eventSendBuffer),
		errorChan: make(chan error, 1),
	}
	e.Lock()
	e.subscribers[name] = sub
	if !e.running {
		shouldStart = true
	}
	e.Unlock()

	return sub.eventChan, sub.errorChan, nil, shouldStart
}

// UnsubscribeFromContainerEvents allows a package to unsubscribe.
// The call is blocking until the request is processed.
func (d *DockerUtil) UnsubscribeFromContainerEvents(name string) error {
	err, shouldStop := d.eventState.unsubscribe(name)

	if shouldStop {
		d.eventState.Lock()
		d.eventState.running = false
		d.eventState.cancelChan <- struct{}{}
		d.eventState.Unlock()
	}

	return err
}

// extracted from UnsubscribeFromContainerEvents for unit testing, additional boolean
// indicates whether the dispatch goroutine should be stopped by DockerUtil
func (e *eventStreamState) unsubscribe(name string) (error, bool) {
	var shouldStop bool
	e.Lock()
	defer e.Unlock()

	sub, found := e.subscribers[name]
	if !found {
		return ErrNotSubscribed, false
	}

	// Remove subscriber
	close(sub.errorChan)
	close(sub.eventChan)
	delete(e.subscribers, name)

	// Stop dispatch if no subs remaining
	if e.running && len(e.subscribers) == 0 {
		shouldStop = true
	}
	return nil, shouldStop
}

func (d *DockerUtil) dispatchEvents(cancelChan <-chan struct{}) {
	fltrs := filters.NewArgs()
	fltrs.Add("type", "container")
	fltrs.Add("event", "start")
	fltrs.Add("event", "die")

	healthTicker := time.NewTicker(15 * time.Second)
	healthToken := health.Register("dockerutil-event-dispatch")

	// Outer loop handles re-connecting in case the docker daemon closes the connection
CONNECT:
	for {
		eventOptions := types.EventsOptions{
			Since:   strconv.FormatInt(time.Now().Unix(), 10),
			Filters: fltrs,
		}

		ctx, cancel := context.WithCancel(context.Background())
		messages, errs := d.cli.Events(ctx, eventOptions)

		// Inner loop iterates over elements in the channel
		for {
			select {
			case <-cancelChan:
				cancel()
				healthTicker.Stop()
				health.Deregister(healthToken)
				return
			case <-healthTicker.C:
				health.Ping(healthToken)
			case err := <-errs:
				if err == io.EOF {
					// Silently ignore io.EOF that happens on http connection reset
					log.Debug("Got EOF, re-connecting")
				} else {
					log.Warnf("error getting docker events: %s", err)
				}
				cancel()
				continue CONNECT // Re-connect to docker
			case msg := <-messages:
				event, err := d.processContainerEvent(msg)
				if err != nil {
					log.Debugf("Skipping event: %s", err)
					continue
				}
				if event == nil {
					continue
				}
				badSubs := d.eventState.dispatch(event)
				for _, sub := range badSubs {
					d.UnsubscribeFromContainerEvents(sub.name)
				}
			}
		}
	}
}

func (e *eventStreamState) dispatch(event *ContainerEvent) []*eventSubscriber {
	var badSubs []*eventSubscriber
	e.RLock()
	for _, sub := range e.subscribers {
		err := e.sendEvent(event, sub)
		if err != nil {
			badSubs = append(badSubs, sub)
		}
	}
	e.RUnlock()
	return badSubs
}

func (s *eventStreamState) sendEvent(ev *ContainerEvent, sub *eventSubscriber) error {
	var err error
	select {
	case sub.eventChan <- ev:
		return nil
	case <-time.After(eventSendTimeout):
		err = ErrEventTimeout
	}

	// We timeouted, let's try to send the error to the subscriber
	select {
	case sub.errorChan <- err:
		// Send successful
	case <-time.After(eventSendTimeout):
		// We don't want to block, let's return
	}
	return err
}
