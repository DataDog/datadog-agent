// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build docker

package docker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	log "github.com/cihub/seelog"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
)

type ContainerEvent struct {
	ContainerID   string
	ContainerName string
	ImageName     string
	Action        string
	Timestamp     time.Time
	Tags          map[string]string
}

// openEventChannel just wraps the client.Event call with saner argument types.
func (d *dockerUtil) openEventChannel(since, until time.Time, filter map[string]string) (<-chan events.Message, <-chan error) {
	// Event since/until string can be formatted or hold a timestamp,
	// see https://github.com/moby/moby/blob/7cbbbb95097f065757d38bcccdb1bbef81d10ddb/api/types/time/timestamp.go#L95
	queryFilter := filters.NewArgs()
	for k, v := range filter {
		queryFilter.Add(k, v)
	}
	options := types.EventsOptions{
		Since:   fmt.Sprintf("%d.%09d", since.Unix(), int64(since.Nanosecond())),
		Until:   fmt.Sprintf("%d.%09d", until.Unix(), int64(until.Nanosecond())),
		Filters: queryFilter,
	}

	return d.cli.Events(context.Background(), options)
}

func (d *dockerUtil) processContainerEvent(msg events.Message) (*ContainerEvent, error) {
	// Type filtering
	if msg.Type != "container" {
		return nil, nil
	}

	// Container filtering
	containerName, found := msg.Actor.Attributes["name"]
	if found == false {
		// TODO: inspect?
		return nil, fmt.Errorf("missing container name in event %s", msg)
	}
	imageName, found := msg.Actor.Attributes["image"]
	if found == false {
		// TODO: inspect?
		return nil, fmt.Errorf("missing image name in event %s", msg)
	}
	if strings.HasPrefix(imageName, "sha256") {
		imageName = d.extractImageName(imageName)
	}
	if d.cfg.filter.computeIsExcluded(containerName, imageName) {
		return nil, nil
	}

	// msg.TimeNano does not hold the nanosecond portion of the timestamp
	// like it's usual to do in Go, but the whole timestamp value as ns value
	// We need to substract the second value to get only the nanoseconds.
	// Keeping that behind a value test in case docker developers fix that
	// inconsistency in the future.
	ns := msg.TimeNano
	if ns > 1e10 {
		ns = ns - msg.Time*1e9
	}

	// Not filtered, return event
	event := &ContainerEvent{
		ContainerID:   msg.Actor.ID,
		ContainerName: containerName,
		ImageName:     imageName,
		Action:        msg.Action,
		Timestamp:     time.Unix(msg.Time, ns),
		Tags:          msg.Actor.Attributes,
	}

	return event, nil
}

// LatestContainerEvents returns events matching the filter that occured after the time passed.
// It returns the latest event timestamp in the slice for the user to store and pass again in the next call.
func (d *dockerUtil) LatestContainerEvents(since time.Time) ([]*ContainerEvent, time.Time, error) {
	var events []*ContainerEvent
	filters := map[string]string{"type": "container"}

	msgChan, errorChan := d.openEventChannel(since, time.Now(), filters)

	var maxTimestamp time.Time

	for {
		select {
		case msg := <-msgChan:
			event, err := d.processContainerEvent(msg)
			if err != nil {
				log.Warnf("error parsing docker message: %s", err)
				continue
			} else if event == nil {
				continue
			}
			events = append(events, event)
			if event.Timestamp.After(maxTimestamp) {
				maxTimestamp = event.Timestamp
			}
		case err := <-errorChan:
			if err == io.EOF {
				break
			} else {
				return events, maxTimestamp, err
			}
		case <-time.After(2 * time.Second):
			log.Warnf("timeout on event receive channel")
			return events, maxTimestamp, errors.New("timeout on event receive channel")
		}
	}
	return events, maxTimestamp, nil
}

// LatestContainerEvents returns events matching the filter that occured after the time passed.
// It returns the latest event timestamp in the slice for the user to store and pass again in the next call.
func LatestContainerEvents(since time.Time) ([]*ContainerEvent, time.Time, error) {
	if globalDockerUtil != nil {
		return globalDockerUtil.LatestContainerEvents(since)
	}
	return nil, time.Now(), errors.New("dockerutil not initialised")
}
