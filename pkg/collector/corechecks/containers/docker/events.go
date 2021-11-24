// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker
// +build docker

package docker

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
)

// reportEvents handles the event retrieval logic
func (d *DockerCheck) retrieveEvents(du docker.Client) ([]*docker.ContainerEvent, error) {
	if d.lastEventTime.IsZero() {
		d.lastEventTime = time.Now().Add(-60 * time.Second)
	}
	events, latest, err := du.LatestContainerEvents(context.TODO(), d.lastEventTime, d.containerFilter)
	if err != nil {
		return events, err
	}

	if latest.IsZero() == false {
		d.lastEventTime = latest.Add(1 * time.Nanosecond)
	}

	return events, nil
}

// reportExitCodes monitors events for non zero exit codes and sends service checks
func (d *DockerCheck) reportExitCodes(events []*docker.ContainerEvent, sender aggregator.Sender) {
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
		if _, ok := d.okExitCodes[int(exitCodeInt)]; !ok {
			status = metrics.ServiceCheckCritical
		}

		tags, err := tagger.Tag(ev.ContainerEntityName(), collectors.HighCardinality)
		if err != nil {
			log.Debugf("no tags for %s: %s", ev.ContainerID, err)
			tags = []string{}
		}

		tags = append(tags, "exit_code:"+exitCodeString)
		sender.ServiceCheck(DockerExit, status, "", tags, message)
	}
}

// reportEvents aggregates and sends events to the Datadog event feed
func (d *DockerCheck) reportEvents(events []*docker.ContainerEvent, sender aggregator.Sender) error {
	bundles := aggregateEvents(events, d.instance.FilteredEventType)

	for _, bundle := range bundles {
		ev, err := bundle.toDatadogEvent(d.dockerHostname)
		if err != nil {
			log.Warnf("can't submit event: %s", err)
		} else {
			sender.Event(ev)
		}
	}
	return nil
}

// aggregateEvents converts a bunch of ContainerEvent to bundles aggregated by
// image name. It also filters out unwanted event types.
func aggregateEvents(events []*docker.ContainerEvent, filteredActions []string) map[string]*dockerEventBundle {
	// Pre-aggregate container events by image
	eventsByImage := make(map[string]*dockerEventBundle)
	filteredByType := make(map[string]int)

	for _, event := range events {
		if matchFilter(event.Action, filteredActions) {
			filteredByType[event.Action] = filteredByType[event.Action] + 1
			continue
		}
		bundle, found := eventsByImage[event.ImageName]
		if found == false {
			bundle = newDockerEventBundler(event.ImageName)
			eventsByImage[event.ImageName] = bundle
		}
		bundle.addEvent(event) //nolint:errcheck
	}

	if len(filteredByType) > 0 {
		log.Debugf("filtered out the following events: %s", formatStringIntMap(filteredByType))
	}
	return eventsByImage
}

func matchFilter(item string, filterList []string) bool {
	for _, filtered := range filterList {
		if filtered == item {
			return true
		}
	}
	return false
}

func formatStringIntMap(input map[string]int) string {
	var parts []string
	for k, v := range input {
		parts = append(parts, fmt.Sprintf("%d %s", v, k))
	}
	return strings.Join(parts, " ")
}
