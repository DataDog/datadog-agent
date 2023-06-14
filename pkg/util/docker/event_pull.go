// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
)

// openEventChannel just wraps the client.Event call with saner argument types.
func (d *DockerUtil) openEventChannel(ctx context.Context, since, until time.Time, filter map[string]string) (<-chan events.Message, <-chan error) {
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

	msgChan, errorChan := d.cli.Events(ctx, options)
	return msgChan, errorChan
}

// processContainerEvent formats the events from a channel.
// It can return nil, nil if the event is filtered out, one should check for nil pointers before using the event.
func (d *DockerUtil) processContainerEvent(ctx context.Context, msg events.Message, filter *containers.Filter) (*ContainerEvent, error) {
	// Type filtering
	// Filtering out prune events as well as they don't have a container name
	if msg.Type != events.ContainerEventType || msg.Action == "prune" {
		return nil, nil
	}

	// Container filtering
	containerName, found := msg.Actor.Attributes["name"]
	if found == false {
		// TODO: inspect?
		m, _ := json.Marshal(msg)
		return nil, fmt.Errorf("missing container name in event %s", string(m))
	}
	imageName, found := msg.Actor.Attributes["image"]
	if found == false {
		// TODO: inspect?
		m, _ := json.Marshal(msg)
		return nil, fmt.Errorf("missing image name in event %s", string(m))
	}
	if strings.HasPrefix(imageName, "sha256") {
		var err error
		imageName, err = d.ResolveImageName(ctx, imageName)
		if err != nil {
			log.Warnf("can't resolve image name %s: %s", imageName, err)
		}
	}
	if filter != nil && filter.IsExcluded(nil, containerName, imageName, "") {
		log.Tracef("events from %s are skipped as the image is excluded for the event collection", containerName)
		return nil, nil
	}

	action := msg.Action

	// Fix the "exec_start: /bin/sh -c true" case
	if strings.Contains(action, ":") {
		action = strings.SplitN(action, ":", 2)[0]
	}

	event := &ContainerEvent{
		ContainerID:   msg.Actor.ID,
		ContainerName: containerName,
		ImageName:     imageName,
		Action:        action,
		Timestamp:     timeFromMessage(msg),
		Attributes:    msg.Actor.Attributes,
	}

	return event, nil
}

// processImageEvent formats the events from a channel.
// It can return nil, nil if the event is filtered out, one should check for nil pointers before using the event.
func (d *DockerUtil) processImageEvent(msg events.Message) *ImageEvent {
	if msg.Type != events.ImageEventType {
		return nil
	}

	return &ImageEvent{
		ImageID:   msg.Actor.ID,
		Action:    msg.Action,
		Timestamp: timeFromMessage(msg),
	}
}

// LatestContainerEvents returns events matching the filter that occurred after the time passed.
// It returns the latest event timestamp in the slice for the user to store and pass again in the next call.
func (d *DockerUtil) LatestContainerEvents(ctx context.Context, since time.Time, filter *containers.Filter) ([]*ContainerEvent, time.Time, error) {
	var containerEvents []*ContainerEvent
	filters := map[string]string{"type": events.ContainerEventType}

	ctx, cancel := context.WithTimeout(ctx, d.queryTimeout)
	defer cancel()
	msgChan, errorChan := d.openEventChannel(ctx, since, time.Now(), filters)

	var maxTimestamp time.Time

	for {
		select {
		case msg := <-msgChan:
			event, err := d.processContainerEvent(ctx, msg, filter)
			if err != nil {
				log.Warnf("error parsing docker message: %s", err)
				continue
			} else if event == nil {
				continue
			}
			containerEvents = append(containerEvents, event)
			if event.Timestamp.After(maxTimestamp) {
				maxTimestamp = event.Timestamp
			}
		case err := <-errorChan:
			if err == io.EOF {
				break
			} else {
				return containerEvents, maxTimestamp, err
			}
		}
	}
}

func timeFromMessage(msg events.Message) time.Time {
	// msg.TimeNano does not hold the nanosecond portion of the timestamp
	// like it's usual to do in Go, but the whole timestamp value as ns value
	// We need to subtract the second value to get only the nanoseconds.
	// Keeping that behind a value test in case docker developers fix that
	// inconsistency in the future.

	ns := msg.TimeNano
	if ns > 1e10 {
		ns = ns - msg.Time*1e9
	}

	return time.Unix(msg.Time, ns)
}
