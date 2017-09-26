// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build docker

package containers

import (
	"errors"
	"fmt"
	"strings"
	"time"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
)

// dockerEventBundle holds a list of ContainerEvent
// identified as coming from the same image. It holds
// the conversion logic to Datadog events for submission
type dockerEventBundle struct {
	imageName     string
	events        []*docker.ContainerEvent
	maxTimestamp  time.Time
	countByAction map[string]int
}

//
func newDockerEventBundler(imageName string) *dockerEventBundle {
	return &dockerEventBundle{
		imageName:     imageName,
		events:        []*docker.ContainerEvent{},
		countByAction: make(map[string]int),
	}
}

func (b *dockerEventBundle) addEvent(event *docker.ContainerEvent) error {
	if event.ImageName != b.imageName {
		return fmt.Errorf("mismatching image name: %s != %s", event.ImageName, b.imageName)
	}
	b.events = append(b.events, event)
	if event.Timestamp.After(b.maxTimestamp) {
		b.maxTimestamp = event.Timestamp
	}
	b.countByAction[event.Action] = b.countByAction[event.Action] + 1

	return nil
}

func (b *dockerEventBundle) toDatadogEvent(hostname string) (metrics.Event, error) {
	output := metrics.Event{
		Priority:       metrics.EventPriorityNormal,
		Host:           hostname,
		SourceTypeName: dockerCheckName,
		EventType:      dockerCheckName,
		Ts:             b.maxTimestamp.Unix(),
		AggregationKey: fmt.Sprintf("docker:%s", b.imageName),
	}
	if len(b.events) == 0 {
		return output, errors.New("no event to export")
	}
	output.Title = fmt.Sprintf("%s %s on %s",
		b.imageName,
		formatStringIntMap(b.countByAction),
		hostname)

	seenContainers := make(map[string]bool)
	textLines := []string{"%%% ", output.Title, "```"}

	for _, ev := range b.events {
		textLines = append(textLines, fmt.Sprintf("%s\t%s", strings.ToUpper(ev.Action), ev.ContainerName))
		seenContainers[ev.ContainerID] = true // Emulating a set with a map
	}
	textLines = append(textLines, "```", " %%%")
	output.Text = strings.Join(textLines, "\n")

	for cid, _ := range seenContainers {
		tags, err := tagger.Tag(fmt.Sprintf("docker://%s", cid), true)
		if err != nil {
			log.Debugf("no tags for %s: %s", cid, err)
		} else {
			output.Tags = append(output.Tags, tags...)
		}
	}

	if b.countByAction["oom"]+b.countByAction["kill"] > 0 {
		output.AlertType = "error"
	}

	return output, nil
}

// reportEvents is the check's entrypoint to retrieve, aggregate and send events
func (d *DockerCheck) reportEvents(sender aggregator.Sender) error {
	if d.lastEventTime.IsZero() {
		d.lastEventTime = time.Now().Add(-60 * time.Second)
	}
	events, latest, err := docker.LatestContainerEvents(d.lastEventTime)
	if err != nil {
		return err
	}

	if latest.IsZero() == false {
		d.lastEventTime = latest.Add(1 * time.Nanosecond)
	}
	bundles, err := d.aggregateEvents(events)

	for _, bundle := range bundles {
		ev, err := bundle.toDatadogEvent(d.dockerHostname)
		if err != nil {
			log.Warnf("can't submit event: %s", err)
		} else {
			ev.Tags = append(ev.Tags, d.instance.Tags...)
			sender.Event(ev)
		}
	}
	return nil
}

// aggregateEvents converts a bunch of ContainerEvent to bundles aggregated by
// image name. It also filters out unwanted event types.
func (d *DockerCheck) aggregateEvents(events []*docker.ContainerEvent) (map[string]*dockerEventBundle, error) {
	// Pre-aggregate container events by image
	eventsByImage := make(map[string]*dockerEventBundle)
	filteredByType := make(map[string]int)

ITER_EVENT:
	for _, event := range events {
		for _, action := range d.instance.FilteredEventType {
			if event.Action == action {
				filteredByType[action] = filteredByType[action] + 1
				continue ITER_EVENT
			}
		}
		bundle, found := eventsByImage[event.ImageName]
		if found == false {
			bundle = newDockerEventBundler(event.ImageName)
			eventsByImage[event.ImageName] = bundle
		}
		bundle.addEvent(event)
	}

	if len(filteredByType) > 0 {
		log.Debugf("filtered out the following events: %s", formatStringIntMap(filteredByType))
	}
	return eventsByImage, nil
}

func formatStringIntMap(input map[string]int) string {
	var parts []string
	for k, v := range input {
		parts = append(parts, fmt.Sprintf("%d %s", v, k))
	}
	return strings.Join(parts, " ")
}
