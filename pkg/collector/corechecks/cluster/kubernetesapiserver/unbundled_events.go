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

func newUnbundledTransformer(clusterName string, taggerInstance tagger.Component, types []collectedEventType) eventTransformer {
	collectedTypes := make([]collectedEventType, 0, len(types))
	for _, f := range types {
		if f.Kind == "" && f.Source == "" {
			log.Errorf(`invalid value for collected_event_types, either "kind" or source" need to be set: %+v`, f)
			continue
		}

		collectedTypes = append(collectedTypes, f)
	}

	return &unbundledTransformer{
		clusterName:        clusterName,
		collectedTypes:     collectedTypes,
		taggerInstance:     taggerInstance,
		bundledTransformer: newBundledTransformer(clusterName, taggerInstance),
	}
}

type unbundledTransformer struct {
	clusterName        string
	collectedTypes     []collectedEventType
	taggerInstance     tagger.Component
	bundledTransformer eventTransformer
}

func (c *unbundledTransformer) Transform(events []*v1.Event) ([]event.Event, []error) {
	var (
		eventsToBundle []*v1.Event
		datadogEvs     []event.Event
		errors         []error
	)

	for _, ev := range events {
		kubeEvents.Inc(
			ev.InvolvedObject.Kind,
			ev.Source.Component,
			ev.Type,
			ev.Reason,
		)

		if !c.shouldCollect(ev) {
			eventsToBundle = append(eventsToBundle, ev)
			continue
		}

		involvedObject := ev.InvolvedObject
		hostInfo := getEventHostInfo(c.clusterName, ev)
		readableKey := buildReadableKey(involvedObject)

		tags := c.buildEventTags(ev, involvedObject, hostInfo)

		emittedEvents.Inc(involvedObject.Kind, ev.Type)
		event := event.Event{
			Title:          fmt.Sprintf("%s: %s", readableKey, ev.Reason),
			Priority:       event.PriorityNormal,
			Host:           hostInfo.hostname,
			SourceTypeName: "kubernetes",
			EventType:      CheckName,
			Ts:             int64(ev.LastTimestamp.Unix()),
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
		fmt.Sprintf("event_reason:%s", ev.Reason),
	)

	// Specific providerID tag
	if hostInfo.providerID != "" {
		tagsAccumulator.Append(fmt.Sprintf("host_provider_id:%s", hostInfo.providerID))
	}

	// Tags from the involved object, including tags from object namespace
	tagsAccumulator.Append(getInvolvedObjectTags(involvedObject, c.taggerInstance)...)

	// Finally tags from the tagger
	c.getTagsFromTagger(involvedObject, tagsAccumulator)

	tagsAccumulator.SortUniq()
	return tagsAccumulator.Get()
}

// getTagsFromTagger add to the TagsAccumulator associated object tags from the tagger.
// For now only Pod object kind is supported.
func (c *unbundledTransformer) getTagsFromTagger(obj v1.ObjectReference, tagsAcc tagset.TagsAccumulator) {
	if c.taggerInstance == nil {
		return
	}

	globalTags, err := c.taggerInstance.GlobalTags(types.HighCardinality)
	if err != nil {
		log.Debugf("error getting global tags: %s", err)
	}
	tagsAcc.Append(globalTags...)

	switch obj.Kind {
	case podKind:
		entityID := fmt.Sprintf("kubernetes_pod_uid://%s", obj.UID)
		entity, err := c.taggerInstance.GetEntity(entityID)
		if err == nil {
			// we can get high Cardinality because tags on events is seemless.
			tagsAcc.Append(entity.GetTags(types.HighCardinality)...)
		} else {
			log.Debugf("error getting pod entity for entity ID: %s, pod tags may be missing", err)
		}

	default:
	}
}

func (c *unbundledTransformer) shouldCollect(ev *v1.Event) bool {
	involvedObject := ev.InvolvedObject

	for _, f := range c.collectedTypes {
		if f.Kind != "" && f.Kind != involvedObject.Kind {
			continue
		}

		if f.Source != "" && f.Source != ev.Source.Component {
			continue
		}

		if len(f.Reasons) == 0 {
			return true
		}

		for _, r := range f.Reasons {
			if ev.Reason == r {
				return true
			}
		}
	}

	return false
}
