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
)

//// eventStreamState logic unit tested in event_stream_test.go
//// DockerUtil logic covered by the listeners/docker and dogstatsd/origin_detection integration tests.
const eventSendBuffer = 5

// SubscribeToContainerEvents allows a package to subscribe to events from the event stream.
// A unique subscriber name should be provided.
func (d *DockerUtil) SubscribeToContainerEvents(name string) (<-chan *ContainerEvent, <-chan error, error) {
	sub, err := d.eventState.subscribe(name)
	if err != nil {
		return nil, nil, err
	}

	go d.dispatchEvents(sub)
	return sub.eventChan, sub.errorChan, err
}

func (e *eventStreamState) subscribe(name string) (*eventSubscriber, error) {
	e.RLock()
	if _, found := e.subscribers[name]; found {
		e.RUnlock()
		return nil, ErrAlreadySubscribed
	}
	e.RUnlock()

	sub := &eventSubscriber{
		name:       name,
		eventChan:  make(chan *ContainerEvent, eventSendBuffer),
		errorChan:  make(chan error, 1), // TODO: remove errorChan once design is stable
		cancelChan: make(chan struct{}),
	}
	e.Lock()
	e.subscribers[name] = sub
	e.Unlock()

	return sub, nil
}

// UnsubscribeFromContainerEvents allows a package to unsubscribe.
// The call is blocking until the request is processed.
func (d *DockerUtil) UnsubscribeFromContainerEvents(name string) error {
	return d.eventState.unsubscribe(name)
}

func (e *eventStreamState) unsubscribe(name string) error {
	e.Lock()
	defer e.Unlock()

	sub, found := e.subscribers[name]
	if !found {
		return ErrNotSubscribed
	}

	// Stop dispatch and remove subscriber
	close(sub.cancelChan)
	delete(e.subscribers, name)
	return nil
}

func (d *DockerUtil) dispatchEvents(sub *eventSubscriber) {
	fltrs := filters.NewArgs()
	fltrs.Add("type", "container")
	fltrs.Add("event", "start")
	fltrs.Add("event", "die")

	// On initial subscribe, don't go back in time. On reconnect, we'll
	// resume at the latest timestamp we got.
	latestTimestamp := time.Now().Unix()
	var cancelFunc context.CancelFunc

CONNECT: // Outer loop handles re-connecting in case the docker daemon closes the connection
	for {
		eventOptions := types.EventsOptions{
			Since:   strconv.FormatInt(latestTimestamp, 10),
			Filters: fltrs,
		}

		var ctx context.Context
		ctx, cancelFunc = context.WithCancel(context.Background())
		messages, errs := d.cli.Events(ctx, eventOptions)

		// Inner loop iterates over elements in the channel
		for {
			select {
			case <-sub.cancelChan:
				break CONNECT
			case err := <-errs:
				if err == io.EOF {
					// Silently ignore io.EOF that happens on http connection reset
					log.Debug("Got EOF, re-connecting")
				} else {
					// Else, let's wait 10 seconds and try reconnecting
					log.Warnf("Got error from docker, waiting for 10 seconds: %s", err)
					time.Sleep(10 * time.Second)
				}
				cancelFunc()
				continue CONNECT // Re-connect to docker
			case msg := <-messages:
				latestTimestamp = msg.Time
				event, err := d.processContainerEvent(msg)
				if err != nil {
					log.Debugf("Skipping event: %s", err)
					continue
				}
				if event == nil {
					continue
				}
				// Block if the buffered channel is full, pausing the http
				// stream. If docker closes because of client timeout, we
				// will reconnect later and stream from latestTimestamp.
				sub.eventChan <- event
			}
		}
	}
	cancelFunc()
	close(sub.errorChan)
	close(sub.eventChan)
}
