// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package docker

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/containers"
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
	alertType     metrics.EventAlertType
}

func newDockerEventBundler(imageName string) *dockerEventBundle {
	return &dockerEventBundle{
		imageName:     imageName,
		events:        []*docker.ContainerEvent{},
		countByAction: make(map[string]int),
		alertType:     metrics.EventAlertTypeInfo,
	}
}

func (b *dockerEventBundle) addEvent(event *docker.ContainerEvent) error {
	if event.ImageName != b.imageName {
		return fmt.Errorf("mismatching image name: %s != %s", event.ImageName, b.imageName)
	}

	b.events = append(b.events, event)
	b.countByAction[event.Action]++

	if event.Timestamp.After(b.maxTimestamp) {
		b.maxTimestamp = event.Timestamp
	}

	if isAlertTypeError(event.Action) {
		b.alertType = metrics.EventAlertTypeError
	}

	return nil
}

func (b *dockerEventBundle) toDatadogEvent(hostname string) (metrics.Event, error) {
	if len(b.events) == 0 {
		return metrics.Event{}, errors.New("no event to export")
	}

	output := metrics.Event{
		Title: fmt.Sprintf("%s %s on %s",
			b.imageName,
			formatStringIntMap(b.countByAction),
			hostname,
		),
		Priority:       metrics.EventPriorityNormal,
		Host:           hostname,
		SourceTypeName: dockerCheckName,
		EventType:      dockerCheckName,
		AlertType:      b.alertType,
		Ts:             b.maxTimestamp.Unix(),
		AggregationKey: fmt.Sprintf("docker:%s", b.imageName),
	}

	seenContainers := make(map[string]bool)
	textLines := []string{"%%% ", output.Title, "```"}

	for _, ev := range b.events {
		textLines = append(textLines, fmt.Sprintf("%s\t%s", strings.ToUpper(ev.Action), ev.ContainerName))
		seenContainers[ev.ContainerID] = true // Emulating a set with a map
	}
	textLines = append(textLines, "```", " %%%")
	output.Text = strings.Join(textLines, "\n")

	for cid := range seenContainers {
		tags, err := tagger.Tag(containers.BuildTaggerEntityName(cid), collectors.HighCardinality)
		if err != nil {
			log.Debugf("no tags for %s: %s", cid, err)
		} else {
			output.Tags = append(output.Tags, tags...)
		}
	}

	return output, nil
}

func formatStringIntMap(input map[string]int) string {
	var parts []string
	for k, v := range input {
		parts = append(parts, fmt.Sprintf("%d %s", v, k))
	}
	return strings.Join(parts, " ")
}
