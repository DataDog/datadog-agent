// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package docker

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func newUnbundledTransformer(hostname string, types []string) eventTransformer {
	collectedEventTypes := make(map[string]struct{}, len(types))
	for _, t := range types {
		collectedEventTypes[t] = struct{}{}
	}

	return &unbundledTransformer{
		hostname:            hostname,
		collectedEventTypes: collectedEventTypes,
	}
}

type unbundledTransformer struct {
	hostname            string
	collectedEventTypes map[string]struct{}
}

func (t *unbundledTransformer) Transform(events []*docker.ContainerEvent) ([]metrics.Event, []error) {
	var (
		datadogEvs []metrics.Event
		errors     []error
	)

	for _, ev := range events {
		if _, ok := t.collectedEventTypes[ev.Action]; !ok {
			continue
		}

		alertType := metrics.EventAlertTypeInfo
		if isAlertTypeError(ev.Action) {
			alertType = metrics.EventAlertTypeError
		}

		emittedEvents.Inc(string(alertType))

		tags, err := tagger.Tag(
			containers.BuildTaggerEntityName(ev.ContainerID),
			collectors.HighCardinality,
		)
		if err != nil {
			log.Debugf("no tags for container %q: %s", ev.ContainerID, err)
		}

		tags = append(tags, fmt.Sprintf("event_type:%s", ev.Action))

		datadogEvs = append(datadogEvs, metrics.Event{
			Title:          fmt.Sprintf("Container %s: %s", ev.ContainerID, ev.Action),
			Text:           fmt.Sprintf("Container %s (running image %q): %s", ev.ContainerID, ev.ImageName, ev.Action),
			Tags:           tags,
			Priority:       metrics.EventPriorityNormal,
			Host:           t.hostname,
			SourceTypeName: dockerCheckName,
			EventType:      dockerCheckName,
			AlertType:      alertType,
			Ts:             ev.Timestamp.Unix(),
			AggregationKey: fmt.Sprintf("docker:%s", ev.ContainerID),
		})
	}

	return datadogEvs, errors
}
