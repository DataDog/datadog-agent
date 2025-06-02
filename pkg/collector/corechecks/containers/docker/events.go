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

	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	dockerEvents = telemetry.NewCounterWithOpts(
		CheckName,
		"events",
		[]string{"action"},
		"Number of Docker events received by the check.",
		telemetry.Options{NoDoubleUnderscoreSep: true},
	)

	emittedEvents = telemetry.NewCounterWithOpts(
		CheckName,
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

	//nolint:gosimple // TODO(CINT) Fix gosimple linter
	if latest.IsZero() == false {
		d.lastEventTime = latest.Add(1 * time.Nanosecond)
	}

	return events, nil
}

// reportExitCodes monitors events for non zero exit codes and sends service checks
func (d *DockerCheck) reportExitCodes(events []*docker.ContainerEvent, sender sender.Sender) {
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
		status := servicecheck.ServiceCheckOK
		if _, ok := d.okExitCodes[int(exitCodeInt)]; !ok {
			status = servicecheck.ServiceCheckCritical
		}

		tags, err := d.tagger.Tag(types.NewEntityID(types.ContainerID, ev.ContainerID), types.HighCardinality)
		if err != nil {
			log.Debugf("no tags for %s: %s", ev.ContainerID, err)
			tags = []string{}
		}

		tags = append(tags, "exit_code:"+exitCodeString)
		sender.ServiceCheck(DockerExit, status, "", tags, message)
	}
}

// reportEvents aggregates and sends events to the Datadog event feed
func (d *DockerCheck) reportEvents(events []*docker.ContainerEvent, sender sender.Sender) error {
	datadogEvs, errs := d.eventTransformer.Transform(events)

	for _, err := range errs {
		d.Warnf("Error transforming events: %s", err.Error()) //nolint:errcheck
	}

	for _, ev := range datadogEvs {
		sender.Event(ev)
	}

	return nil
}
