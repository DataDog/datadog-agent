// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package docker

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	dockerEvents = telemetry.NewCounterWithOpts(
		dockerCheckName,
		"events",
		[]string{"action"},
		"Number of Docker events received by the check.",
		telemetry.Options{NoDoubleUnderscoreSep: true},
	)

	emittedEvents = telemetry.NewCounterWithOpts(
		dockerCheckName,
		"emitted_events",
		[]string{"type"},
		"Number of events emitted by the check.",
		telemetry.Options{NoDoubleUnderscoreSep: true},
	)
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

		tags, err := tagger.Tag(containers.BuildTaggerEntityName(ev.ContainerID), collectors.HighCardinality)
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
	datadogEvs, errs := d.eventTransformer.Transform(events)

	for _, err := range errs {
		d.Warnf("Error transforming events: %s", err.Error()) //nolint:errcheck
	}

	for _, ev := range datadogEvs {
		sender.Event(ev)
	}

	return nil
}
