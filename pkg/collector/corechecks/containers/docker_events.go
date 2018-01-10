// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build docker

package containers

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
)

// reportEvents handles the event retrieval logic
func (d *DockerCheck) retrieveEvents(du *docker.DockerUtil) ([]*docker.ContainerEvent, error) {
	if d.lastEventTime.IsZero() {
		d.lastEventTime = time.Now().Add(-60 * time.Second)
	}
	events, latest, err := du.LatestContainerEvents(d.lastEventTime)
	if err != nil {
		return events, err
	}

	if latest.IsZero() == false {
		d.lastEventTime = latest.Add(1 * time.Nanosecond)
	}

	return events, nil
}

// reportExitCodes monitors events for non zero exit codes and sends service checks
func (d *DockerCheck) reportExitCodes(events []*docker.ContainerEvent, sender aggregator.Sender) error {
	for _, ev := range events {
		// Filtering
		if ev.Action != "die" {
			continue
		}
		exitCodeString, codeFound := ev.Attributes["exitCode"]
		if !codeFound {
			log.Warnf("skipping event with no exit code: %s", ev)
			continue
		}
		exitCodeInt, err := strconv.ParseInt(exitCodeString, 10, 32)
		if err != nil {
			log.Warnf("skipping event with invalid exit code: %s", err.Error())
			continue
		}

		// Building and sending message
		message := fmt.Sprintf("Container %s exited with %d", ev.ContainerName, exitCodeInt)
		status := metrics.ServiceCheckOK
		if exitCodeInt != 0 {
			status = metrics.ServiceCheckCritical
		}
		tags, err := tagger.Tag(ev.ContainerEntityName(), true)
		tags = append(tags, d.instance.Tags...)
		if err != nil {
			log.Debugf("no tags for %s: %s", ev.ContainerID, err)
		}
		sender.ServiceCheck(DockerExit, status, "", tags, message)
	}

	return nil
}

// reportEvents aggregates and sends events to the Datadog event feed
func (d *DockerCheck) reportEvents(events []*docker.ContainerEvent, sender aggregator.Sender) error {
	bundles, err := d.aggregateEvents(events)
	if err != nil {
		return err
	}

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
