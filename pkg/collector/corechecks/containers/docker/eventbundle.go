// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker
// +build docker

package docker

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
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

	for cid := range seenContainers {
		tags, err := tagger.Tag(docker.ContainerIDToTaggerEntityName(cid), collectors.HighCardinality)
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
