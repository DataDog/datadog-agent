// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package docker

import (
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func newUnbundledTransformer(hostname string, types []string, bundledTransformer eventTransformer, tagger tagger.Component) eventTransformer {
	collectedEventTypes := make(map[string]struct{}, len(types))
	for _, t := range types {
		collectedEventTypes[t] = struct{}{}
	}

	return &unbundledTransformer{
		hostname:            hostname,
		collectedEventTypes: collectedEventTypes,
		bundledTransformer:  bundledTransformer,
		tagger:              tagger,
	}
}

type unbundledTransformer struct {
	hostname            string
	collectedEventTypes map[string]struct{}
	bundledTransformer  eventTransformer
	tagger              tagger.Component
}

func (t *unbundledTransformer) Transform(events []*docker.ContainerEvent) ([]event.Event, []error) {
	var (
		datadogEvs  []event.Event
		errors      []error
		evsToBundle []*docker.ContainerEvent
	)

	for _, ev := range events {
		if _, ok := t.collectedEventTypes[string(ev.Action)]; !ok {
			evsToBundle = append(evsToBundle, ev)
			continue
		}

		alertType := event.AlertTypeInfo
		if isAlertTypeError(ev.Action) {
			alertType = event.AlertTypeError
		}

		emittedEvents.Inc(string(alertType))

		tags, err := t.tagger.Tag(
			types.NewEntityID(types.ContainerID, ev.ContainerID),
			types.HighCardinality,
		)
		if err != nil {
			log.Debugf("no tags for container %q: %s", ev.ContainerID, err)
		}

		tags = append(tags, fmt.Sprintf("event_type:%s", ev.Action))

		datadogEvs = append(datadogEvs, event.Event{
			Title:          fmt.Sprintf("Container %s: %s", ev.ContainerID, ev.Action),
			Text:           fmt.Sprintf("Container %s (running image %q): %s", ev.ContainerID, ev.ImageName, ev.Action),
			Tags:           tags,
			Priority:       event.PriorityNormal,
			Host:           t.hostname,
			SourceTypeName: CheckName,
			EventType:      CheckName,
			AlertType:      alertType,
			Ts:             ev.Timestamp.Unix(),
			AggregationKey: fmt.Sprintf("docker:%s", ev.ContainerID),
		})
	}

	bundledEvs, errs := t.bundledTransformer.Transform(evsToBundle)

	return append(datadogEvs, bundledEvs...), append(errors, errs...)
}
