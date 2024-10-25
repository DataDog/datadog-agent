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

	"github.com/docker/docker/api/types/events"

	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// dockerEventBundle holds a list of ContainerEvent
// identified as coming from the same image. It holds
// the conversion logic to Datadog events for submission
type dockerEventBundle struct {
	imageName     string
	events        []*docker.ContainerEvent
	maxTimestamp  time.Time
	countByAction map[events.Action]int
	alertType     event.AlertType
	tagger        tagger.Component
}

func newDockerEventBundler(imageName string, tagger tagger.Component) *dockerEventBundle {
	return &dockerEventBundle{
		imageName:     imageName,
		events:        []*docker.ContainerEvent{},
		countByAction: make(map[events.Action]int),
		alertType:     event.AlertTypeInfo,
		tagger:        tagger,
	}
}

func (b *dockerEventBundle) addEvent(ev *docker.ContainerEvent) error {
	if ev.ImageName != b.imageName {
		return fmt.Errorf("mismatching image name: %s != %s", ev.ImageName, b.imageName)
	}

	b.events = append(b.events, ev)
	b.countByAction[ev.Action]++

	if ev.Timestamp.After(b.maxTimestamp) {
		b.maxTimestamp = ev.Timestamp
	}

	if isAlertTypeError(ev.Action) {
		b.alertType = event.AlertTypeError
	}

	return nil
}

func (b *dockerEventBundle) toDatadogEvent(hostname string) (event.Event, error) {
	if len(b.events) == 0 {
		return event.Event{}, errors.New("no event to export")
	}

	output := event.Event{
		Title: fmt.Sprintf("%s %s on %s",
			b.imageName,
			formatActionMap(b.countByAction),
			hostname,
		),
		Priority:       event.PriorityNormal,
		Host:           hostname,
		SourceTypeName: CheckName,
		EventType:      CheckName,
		AlertType:      b.alertType,
		Ts:             b.maxTimestamp.Unix(),
		AggregationKey: fmt.Sprintf("docker:%s", b.imageName),
	}

	seenContainers := make(map[string]bool)
	textLines := []string{"%%% ", output.Title, "```"}

	for _, ev := range b.events {
		textLines = append(textLines, fmt.Sprintf("%s\t%s", strings.ToUpper(string(ev.Action)), ev.ContainerName))
		seenContainers[ev.ContainerID] = true // Emulating a set with a map
	}
	textLines = append(textLines, "```", " %%%")
	output.Text = strings.Join(textLines, "\n")

	for cid := range seenContainers {
		tags, err := b.tagger.Tag(types.NewEntityID(types.ContainerID, cid), types.HighCardinality)
		if err != nil {
			log.Debugf("no tags for %s: %s", cid, err)
		} else {
			output.Tags = append(output.Tags, tags...)
		}
	}

	return output, nil
}

func formatActionMap(input map[events.Action]int) string {
	var parts []string
	for k, v := range input {
		parts = append(parts, fmt.Sprintf("%d %s", v, k))
	}
	return strings.Join(parts, " ")
}
