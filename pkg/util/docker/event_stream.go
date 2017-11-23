// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build docker

package docker

import (
	"context"
	"fmt"
	"time"

	log "github.com/cihub/seelog"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"

	"github.com/DataDog/datadog-agent/pkg/util/fanout"
)

func (d *DockerUtil) SubscribeToContainerEvents(name string) (<-chan *ContainerEvent, <-chan error, error) {
	if d.fanner == nil {
		d.stopEventStream = make(chan struct{})
		d.fanner = &eventFanout{}
		dataChan, errChan, err := d.fanner.Setup(fanout.Config{
			Name:             "docker_events",
			WriteTimeout:     time.Second,
			OutputBufferSize: 50,
		})

		if err != nil {
			return nil, nil, err
		}
		go d.streamEvents(dataChan, errChan)
	}

	return d.fanner.Suscribe(name)
}

func (d *DockerUtil) UnsuscribeFromContainerEvents(name string) error {
	last, err := d.fanner.Unsuscribe(name)
	if err != nil {
		return err
	}
	if last {
		d.stopEventStream <- struct{}{} // Blocking until steamEvents returns
		d.fanner = nil
	}
	return nil
}

func (d *DockerUtil) streamEvents(dataChan chan<- *ContainerEvent, errChan chan<- error) {
	filters := filters.NewArgs()
	filters.Add("type", "container")
	filters.Add("event", "start")
	filters.Add("event", "die")

	// First loop handles re-connecting in case the docker daemon closes the connection
	for {
		eventOptions := types.EventsOptions{
			Since:   fmt.Sprintf("%d", time.Now().Unix()),
			Filters: filters,
		}

		ctx, cancel := context.WithCancel(context.Background())
		messages, errs := d.cli.Events(ctx, eventOptions)

	TRANSMIT: // Second loop iterates over elements in the channel
		for {

			select {
			case <-d.stopEventStream:
				cancel()
				return
			case msg := <-messages:
				event, err := d.processContainerEvent(msg)
				if err != nil {
					log.Debugf("skipping event: %s", err)
				}
				dataChan <- event
			case err := <-errs:
				log.Warnf("error getting docker events: %s", err)
				break TRANSMIT
			}
		}
		cancel()
	}
}
