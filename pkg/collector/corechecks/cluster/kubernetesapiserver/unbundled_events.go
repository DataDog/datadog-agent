// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubernetesapiserver

import (
	"fmt"

	v1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func newUnbundledTransformer(clusterName string, taggerInstance tagger.Tagger, types []collectedEventType) eventTransformer {
	collectedTypes := make([]collectedEventType, 0, len(types))
	for _, f := range types {
		if f.Kind == "" && f.Source == "" {
			log.Errorf(`invalid value for collected_event_types, either "kind" or source" need to be set: %+v`, f)
			continue
		}

		collectedTypes = append(collectedTypes, f)
	}

	return &unbundledTransformer{
		clusterName:    clusterName,
		collectedTypes: collectedTypes,
		taggerInstance: tagger.GetDefaultTagger(),
	}
}

type unbundledTransformer struct {
	clusterName    string
	collectedTypes []collectedEventType
	taggerInstance tagger.Tagger
}

func (c *unbundledTransformer) Transform(events []*v1.Event) ([]metrics.Event, []error) {
	var (
		datadogEvs []metrics.Event
		errors     []error
	)

	for _, ev := range events {
		kubeEvents.Inc(
			ev.InvolvedObject.Kind,
			ev.Source.Component,
			ev.Type,
			ev.Reason,
		)

		if !c.shouldCollect(ev) {
			continue
		}

		involvedObject := ev.InvolvedObject
		hostInfo := getEventHostInfo(c.clusterName, ev)
		readableKey := buildReadableKey(involvedObject)
		tagsAccumulator := tagset.NewHashlessTagsAccumulator()

		tagsAccumulator.Append(getInvolvedObjectTags(involvedObject)...)
		tagsAccumulator.Append(
			fmt.Sprintf("source_component:%s", ev.Source.Component),
			fmt.Sprintf("event_reason:%s", ev.Reason))

		if hostInfo.providerID != "" {
			tagsAccumulator.Append(fmt.Sprintf("host_provider_id:%s", hostInfo.providerID))
		}
		c.getTagsFromTagger(involvedObject, tagsAccumulator)
		tagsAccumulator.SortUniq()

		emittedEvents.Inc(involvedObject.Kind, ev.Type)

		datadogEvs = append(datadogEvs, metrics.Event{
			Title:          fmt.Sprintf("%s: %s", readableKey, ev.Reason),
			Priority:       metrics.EventPriorityNormal,
			Host:           hostInfo.hostname,
			SourceTypeName: "kubernetes",
			EventType:      kubernetesAPIServerCheckName,
			Ts:             int64(ev.LastTimestamp.Unix()),
			Tags:           tagsAccumulator.Get(),
			AggregationKey: fmt.Sprintf("kubernetes_apiserver:%s", involvedObject.UID),
			AlertType:      getDDAlertType(ev.Type),
			Text:           ev.Message,
		})
	}

	return datadogEvs, errors
}

// getTagsFromTagger add to the TagsAccumulator associated object tags from the tagger.
// For now only Pod object kind is supported.
func (c *unbundledTransformer) getTagsFromTagger(obj v1.ObjectReference, tagsAcc tagset.TagsAccumulator) {
	if c.taggerInstance == nil {
		return
	}
	switch obj.Kind {
	case podKind:
		entityId := fmt.Sprintf("kubernetes_pod_uid://%s", obj.UID)
		entity, err := c.taggerInstance.GetEntity(entityId)
		if err != nil {
			return
		}
		// we can get high Cardinality because tags on events is seemless.
		tagsAcc.Append(entity.GetTags(collectors.HighCardinality)...)
	default:
		return
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
