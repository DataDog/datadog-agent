// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubernetesapiserver

import (
	"fmt"

	v1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func newUnbundledTransformer(clusterName string, taggerInstance tagger.Component, types []collectedEventType, bundleUnspecifiedEvents bool, filteringEnabled bool) eventTransformer {
	collectedTypes := make([]collectedEventType, 0, len(types))
	for _, f := range types {
		if f.Kind == "" && f.Source == "" {
			log.Errorf(`invalid value for collected_event_types, either "kind" or source" need to be set: %+v`, f)
			continue
		}

		collectedTypes = append(collectedTypes, f)
	}

	var t eventTransformer = noopEventTransformer{}
	if bundleUnspecifiedEvents {
		t = newBundledTransformer(clusterName, taggerInstance, collectedTypes, false)
	}

	return &unbundledTransformer{
		clusterName:             clusterName,
		collectedTypes:          collectedTypes,
		taggerInstance:          taggerInstance,
		bundledTransformer:      t,
		bundleUnspecifiedEvents: bundleUnspecifiedEvents,
		filteringEnabled:        filteringEnabled,
	}
}

type unbundledTransformer struct {
	clusterName             string
	collectedTypes          []collectedEventType
	taggerInstance          tagger.Component
	bundledTransformer      eventTransformer
	bundleUnspecifiedEvents bool
	filteringEnabled        bool
}

func (c *unbundledTransformer) Transform(events []*v1.Event) ([]event.Event, []error) {
	var (
		eventsToBundle []*v1.Event
		datadogEvs     []event.Event
		errors         []error
	)

	for _, ev := range events {

		source := getEventSource(ev.ReportingController, ev.Source.Component)

		kubeEvents.Inc(
			ev.InvolvedObject.Kind,
			ev.Source.Component,
			ev.Type,
			ev.Reason,
			source,
		)

		collectedByDefault := false
		if c.filteringEnabled {
			if !shouldCollectByDefault(ev) {
				source = fmt.Sprintf("%s_%s", source, customEventSourceSuffix)
			} else {
				collectedByDefault = true
			}
		}

		isCollected := collectedByDefault || c.shouldCollect(ev)
		if !isCollected {
			if c.bundleUnspecifiedEvents {
				eventsToBundle = append(eventsToBundle, ev)
			}
			continue
		}

		involvedObject := ev.InvolvedObject
		hostInfo := getEventHostInfo(c.clusterName, ev)
		readableKey := buildReadableKey(involvedObject)

		tags := c.buildEventTags(ev, involvedObject, hostInfo)

		emittedEvents.Inc(
			involvedObject.Kind,
			ev.Type,
			source,
			"false",
		)

		var timestamp int64
		if ev.LastTimestamp.IsZero() {
			timestamp = int64(ev.EventTime.Unix())
		} else {
			timestamp = int64(ev.LastTimestamp.Unix())
		}

		event := event.Event{
			Title:          fmt.Sprintf("%s: %s", readableKey, ev.Reason),
			Priority:       event.PriorityNormal,
			Host:           hostInfo.hostname,
			SourceTypeName: source,
			EventType:      CheckName,
			Ts:             timestamp,
			Tags:           tags,
			AggregationKey: fmt.Sprintf("kubernetes_apiserver:%s", involvedObject.UID),
			AlertType:      getDDAlertType(ev.Type),
			Text:           ev.Message,
		}
		datadogEvs = append(datadogEvs, event)
	}

	bundledEvents, errs := c.bundledTransformer.Transform(eventsToBundle)

	return append(datadogEvs, bundledEvents...), append(errors, errs...)
}

// buildEventTags aggregate all tags for an event from multiple sources.
func (c *unbundledTransformer) buildEventTags(ev *v1.Event, involvedObject v1.ObjectReference, hostInfo eventHostInfo) []string {
	tagsAccumulator := tagset.NewHashlessTagsAccumulator()

	// Hardcoded tags
	tagsAccumulator.Append(
		fmt.Sprintf("source_component:%s", ev.Source.Component),
		"orchestrator:kubernetes",
		fmt.Sprintf("reporting_controller:%s", ev.ReportingController),
		fmt.Sprintf("event_reason:%s", ev.Reason),
	)

	// Specific providerID tag
	if hostInfo.providerID != "" {
		tagsAccumulator.Append(fmt.Sprintf("host_provider_id:%s", hostInfo.providerID))
	}

	// Tags from the involved object, including tags from object namespace
	tagsAccumulator.Append(getInvolvedObjectTags(involvedObject, c.taggerInstance)...)

	// Finally tags from the tagger
	c.getTagsFromTagger(tagsAccumulator)

	tagsAccumulator.SortUniq()
	return tagsAccumulator.Get()
}

// getTagsFromTagger add to the TagsAccumulator global tags from the tagger
func (c *unbundledTransformer) getTagsFromTagger(tagsAcc tagset.TagsAccumulator) {
	if c.taggerInstance == nil {
		return
	}

	globalTags, err := c.taggerInstance.GlobalTags(types.HighCardinality)
	if err != nil {
		log.Debugf("error getting global tags: %s", err)
	}
	tagsAcc.Append(globalTags...)
}

func (c *unbundledTransformer) shouldCollect(ev *v1.Event) bool {
	return shouldCollect(ev, c.collectedTypes)
}
