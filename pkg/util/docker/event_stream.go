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
	d.eventState.RLock()
	if _, found := d.eventState.subscribers[name]; found {
		d.eventState.RUnlock()
		return nil, nil, ErrAlreadySubscribed
	}
	d.eventState.RUnlock()

	sub := &eventSubscriber{
		name:      name,
		eventChan: make(chan *ContainerEvent, 5),
		errorChan: make(chan error, 1),
	}
	d.eventState.Lock()
	d.eventState.subscribers[name] = sub
	if !d.eventState.running {
		d.eventState.running = true
		go d.dispatchEvents()
	}
	d.eventState.Unlock()

	return sub.eventChan, sub.errorChan, nil
}

// UnsubscribeFromContainerEvents allows a package to unsubscribe.
// The call is blocking until the request is processed.
func (d *DockerUtil) UnsubscribeFromContainerEvents(name string) error {
	d.eventState.Lock()
	sub, found := d.eventState.subscribers[name]
	if !found {
		d.eventState.Unlock()
		return ErrNotSubscribed
	}

	// Remove subscriber
	close(sub.errorChan)
	close(sub.eventChan)
	delete(d.eventState.subscribers, name)

	// Stop dispatch if no subs remaining
	if d.eventState.running && len(d.eventState.subscribers) == 0 {
		d.eventState.cancelChan <- struct{}{}
	}
	d.eventState.Unlock()
	return nil
}

func (d *DockerUtil) dispatchEvents() {
	fltrs := filters.NewArgs()
	fltrs.Add("type", "container")
	fltrs.Add("event", "start")
	fltrs.Add("event", "die")

	badSubs := make(map[*eventSubscriber]error)

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
			case <-d.eventState.cancelChan:
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

				d.eventState.RLock()
				for _, sub := range d.eventState.subscribers {
					err := sub.sendEvent(event)
					if err != nil {
						badSubs[sub] = err
					}
				}
				d.eventState.RUnlock()

				if len(badSubs) > 0 {
					for sub, err := range badSubs {
						log.Infof("forcefully unsuscribing %s from: %s", sub.name, err)
						sub.sendError(err)
						d.UnsubscribeFromContainerEvents(sub.name)
					}
					badSubs = make(map[*eventSubscriber]error)
				}
			}
		}
	}
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
