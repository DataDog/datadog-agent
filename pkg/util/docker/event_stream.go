// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build docker

package docker

import (
	"context"
	"errors"
	"io"
	"strconv"
	"time"

	log "github.com/cihub/seelog"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
)

//// Can't be unit tested, covered by the listeners/docker
//// and dogstatsd/origin_detection integration tests.

// eventSubscriber holds the state for a subscriber
type eventSubscriber struct {
	name       string
	eventChan  chan *ContainerEvent
	errorChan  chan error
	cancelChan chan struct{}
}

// SubscribeToContainerEvents allows a package to subscribe to events from the event stream.
// An unique subscriber name should be provided.
func (d *DockerUtil) SubscribeToContainerEvents(name string) (<-chan *ContainerEvent, <-chan error, error) {
	d.RLock()
	if _, found := d.eventSubscribers[name]; found {
		d.RUnlock()
		return nil, nil, errors.New("already subscribed")
	}
	d.RUnlock()

	sub := &eventSubscriber{
		name:       name,
		eventChan:  make(chan *ContainerEvent, 5),
		errorChan:  make(chan error, 5),
		cancelChan: make(chan struct{}),
	}
	d.Lock()
	d.eventSubscribers[name] = sub
	d.Unlock()

	go d.streamEvents(sub.eventChan, sub.cancelChan)
	return sub.eventChan, sub.errorChan, nil
}

// UnsubscribeFromContainerEvents allows a package to unsubscribe.
// The call is blocking until the request is processed.
func (d *DockerUtil) UnsubscribeFromContainerEvents(name string) error {
	d.Lock()
	sub, found := d.eventSubscribers[name]
	if !found {
		d.Unlock()
		return errors.New("not subscribed")
	}
	delete(d.eventSubscribers, name)
	d.Unlock()

	// Block until the goroutine exits, then close all chans
	sub.cancelChan <- struct{}{}
	close(sub.cancelChan)
	close(sub.errorChan)
	close(sub.eventChan)

	return nil
}

func (d *DockerUtil) streamEvents(dataChan chan<- *ContainerEvent, cancelChan <-chan struct{}) {
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
			case msg := <-messages:
				event, err := d.processContainerEvent(msg)
				if err != nil {
					log.Debugf("skipping event: %s", err)
					continue
				}
				dataChan <- event
			case err := <-errs:
				if err == io.EOF {
					// Silently ignore io.EOF that happens on http connection reset
					log.Debug("got EOF, re-connecting")
				} else {
					log.Warnf("error getting docker events: %s", err)
				}
				cancel()
				continue CONNECT // Re-connect to docker
			}
		}
	}
}
