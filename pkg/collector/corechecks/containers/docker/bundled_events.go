// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package docker

import (
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// newBundledTransformer returns a transformer that bundles together docker
// events from containers that share the same image. that's a behavior that no
// longer makes sense, and we'd like to remove it in Agent 8.
func newBundledTransformer(hostname string, types []string, tagger tagger.Component) eventTransformer {
	filteredEventTypes := make(map[string]struct{}, len(types))
	for _, t := range types {
		filteredEventTypes[t] = struct{}{}
	}

	return &bundledTransformer{
		hostname:           hostname,
		filteredEventTypes: filteredEventTypes,
		tagger:             tagger,
	}
}

type bundledTransformer struct {
	hostname           string
	filteredEventTypes map[string]struct{}
	tagger             tagger.Component
}

func (t *bundledTransformer) Transform(events []*docker.ContainerEvent) ([]event.Event, []error) {
	var errs []error

	bundles := t.aggregateEvents(events)
	datadogEvs := make([]event.Event, 0, len(bundles))

	for _, bundle := range bundles {
		ddEv, err := bundle.toDatadogEvent(t.hostname)
		if err != nil {
			errs = append(errs, err)
		}

		datadogEvs = append(datadogEvs, ddEv)

		emittedEvents.Inc(string(bundle.alertType))
	}

	return datadogEvs, errs
}

// aggregateEvents converts a bunch of ContainerEvent to bundles aggregated by
// image name. It also filters out unwanted event types.
func (t *bundledTransformer) aggregateEvents(events []*docker.ContainerEvent) map[string]*dockerEventBundle {
	eventsByImage := make(map[string]*dockerEventBundle)

	for _, event := range events {
		dockerEvents.Inc(string(event.Action))

		if _, ok := t.filteredEventTypes[string(event.Action)]; ok {
			continue
		}

		bundle, found := eventsByImage[event.ImageName]
		if !found {
			bundle = newDockerEventBundler(event.ImageName, t.tagger)
			eventsByImage[event.ImageName] = bundle
		}

		err := bundle.addEvent(event)
		if err != nil {
			log.Warnf("Error while bundling events, %s.", err.Error())
			continue
		}
	}

	return eventsByImage
}
