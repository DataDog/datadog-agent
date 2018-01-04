// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

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
)

//// Can't be unit tested, covered by the listeners/docker
//// and dogstatsd/origin_detection integration tests.

const eventSendTimeout = 100 * time.Millisecond

// SubscribeToContainerEvents allows a package to subscribe to events from the event stream.
// A unique subscriber name should be provided.
func (d *DockerUtil) SubscribeToContainerEvents(name string) (<-chan *ContainerEvent, <-chan error, error) {
	c1, c2, err, shouldStart := d.eventState.subscribe(name)

	if shouldStart {
		d.eventState.Lock()
		d.eventState.running = true
		go d.dispatchEvents(d.eventState.cancelChan)
		d.eventState.Unlock()
	}

	return c1, c2, err
}

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
		eventChan: make(chan *ContainerEvent, 5),
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

func (e *eventStreamState) unsubscribe(name string) (error, bool) {
	var shouldStop bool
	e.Lock()
	sub, found := e.subscribers[name]
	if !found {
		e.Unlock()
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
	e.Unlock()
	return nil, shouldStop
}

func (d *DockerUtil) dispatchEvents(cancelChan <-chan struct{}) {
	fltrs := filters.NewArgs()
	fltrs.Add("type", "container")
	fltrs.Add("event", "start")
	fltrs.Add("event", "die")

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
				return
			case err := <-errs:
				if err == io.EOF {
					// Silently ignore io.EOF that happens on http connection reset
					log.Debug("got EOF, re-connecting")
				} else {
					log.Warnf("error getting docker events: %s", err)
				}
				cancel()
				continue CONNECT // Re-connect to docker
			case msg := <-messages:
				event, err := d.processContainerEvent(msg)
				if err != nil {
					log.Debugf("skipping event: %s", err)
					continue
				}
				badSubs := d.eventState.dispatch(event)
				if len(badSubs) > 0 {
					for _, sub := range badSubs {
						d.UnsubscribeFromContainerEvents(sub.name)
					}
				}
			}
		}
	}
}

func (e *eventStreamState) dispatch(event *ContainerEvent) []*eventSubscriber {
	var badSubs []*eventSubscriber
	e.RLock()
	for _, sub := range e.subscribers {
		err := sub.sendEvent(event)
		if err != nil {
			badSubs = append(badSubs, sub)
			sub.sendError(err)
		}
	}
	e.RUnlock()
	return badSubs
}

func (s *eventSubscriber) sendEvent(ev *ContainerEvent) error {
	select {
	case s.eventChan <- ev:
		return nil
	case <-time.After(eventSendTimeout):
		return ErrEventTimeout
	}
}

func (s *eventSubscriber) sendError(err error) error {
	select {
	case s.errorChan <- err:
		return nil
	case <-time.After(eventSendTimeout):
		return ErrEventTimeout
	}
}
