// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubernetesapiserver

import (
	v1 "k8s.io/api/core/v1"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
)

func newBundledTransformer(clusterName string, taggerInstance tagger.Component, collectedTypes []collectedEventType, filteringEnabled bool) eventTransformer {
	return &bundledTransformer{
		clusterName:      clusterName,
		taggerInstance:   taggerInstance,
		collectedTypes:   collectedTypes,
		filteringEnabled: filteringEnabled,
	}
}

type bundledTransformer struct {
	clusterName      string
	taggerInstance   tagger.Component
	collectedTypes   []collectedEventType
	filteringEnabled bool
}

func (c *bundledTransformer) Transform(events []*v1.Event) ([]event.Event, []error) {
	var errors []error

	// Map bundleID to a slice of kubernetesEventBundles, as we can have several bundles for the same object if the event text length exceeds the maximum allowed length.
	bundlesByObject := make(map[bundleID][]*kubernetesEventBundle)

	for _, event := range events {
		if event.InvolvedObject.Kind == "" ||
			event.InvolvedObject.Name == "" ||
			event.Reason == "" ||
			event.Message == "" {
			continue
		}

		kubeEvents.Inc(
			event.InvolvedObject.Kind,
			event.Source.Component,
			event.Type,
			event.Reason,
			getEventSource(event.ReportingController, event.Source.Component),
		)

		if c.filteringEnabled {
			if !(shouldCollectByDefault(event) || shouldCollect(event, c.collectedTypes)) {
				continue
			}
		}

		id := buildBundleID(event)
		bundles := bundlesByObject[id]

		// Add the event to the last bundle for the object when it fits.
		if len(bundles) > 0 {
			lastBundle := bundles[len(bundles)-1]
			if _, fits := lastBundle.fitsEvent(event); fits {
				if err := lastBundle.addEvent(event); err != nil {
					errors = append(errors, err)
				}
				continue
			}
		}

		// Start a new bundle. Register it only once the event is added, so an event
		// too large for any bundle is dropped without leaving an empty bundle behind
		// (empty bundles fail to export).
		newBundle := newKubernetesEventBundler(c.clusterName, event)
		if err := newBundle.addEvent(event); err != nil {
			errors = append(errors, err)
			continue
		}
		bundlesByObject[id] = append(bundles, newBundle)
	}

	datadogEvs := make([]event.Event, 0, len(bundlesByObject))

	for id, bundles := range bundlesByObject {
		for _, bundle := range bundles {
			datadogEv, err := bundle.formatEvents(c.taggerInstance)
			if err != nil {
				errors = append(errors, err)
				continue
			}

			emittedEvents.Inc(
				id.kind,
				id.evType,
				getEventSource(bundle.reportingController, bundle.component),
				"true",
			)

			datadogEvs = append(datadogEvs, datadogEv)
		}
	}

	return datadogEvs, errors
}

type bundleID struct {
	kind   string
	uid    string
	evType string
}

// buildBundleID generates a unique ID to separate k8s events
// based on their InvolvedObject UIDs and event Types
func buildBundleID(e *v1.Event) bundleID {
	return bundleID{
		kind:   e.InvolvedObject.Kind,
		uid:    string(e.InvolvedObject.UID),
		evType: e.Type,
	}
}
