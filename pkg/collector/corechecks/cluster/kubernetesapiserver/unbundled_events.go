// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver
// +build kubeapiserver

package kubernetesapiserver

import (
	"fmt"
	"strings"

	v1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func newUnbundledTransformer(clusterName string, types map[string][]string) eventTransformer {
	collectedTypes := make(map[string]map[string]struct{}, len(types))
	for kind, reasons := range types {
		collectedTypes[strings.ToLower(kind)] = make(map[string]struct{}, len(reasons))
		for _, reason := range reasons {
			collectedTypes[strings.ToLower(kind)][reason] = struct{}{}
		}
	}

	return &unbundledTransformer{
		clusterName:    clusterName,
		collectedTypes: collectedTypes,
	}
}

type unbundledTransformer struct {
	clusterName    string
	collectedTypes map[string]map[string]struct{}
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

		tags := getInvolvedObjectTags(involvedObject)
		tags = append(tags,
			fmt.Sprintf("source_component:%s", ev.Source.Component),
			fmt.Sprintf("event_reason:%s", ev.Reason),
		)

		if hostInfo.providerID != "" {
			tags = append(tags, fmt.Sprintf("host_provider_id:%s", hostInfo.providerID))
		}

		emittedEvents.Inc(involvedObject.Kind, ev.Type)

		datadogEvs = append(datadogEvs, metrics.Event{
			Title:          fmt.Sprintf("%s: %s", readableKey, ev.Reason),
			Priority:       metrics.EventPriorityNormal,
			Host:           hostInfo.hostname,
			SourceTypeName: "kubernetes",
			EventType:      kubernetesAPIServerCheckName,
			Ts:             int64(ev.LastTimestamp.Unix()),
			Tags:           tags,
			AggregationKey: fmt.Sprintf("kubernetes_apiserver:%s", involvedObject.UID),
			AlertType:      getDDAlertType(ev.Type),
			Text:           ev.Message,
		})
	}

	return datadogEvs, errors
}

func (c *unbundledTransformer) shouldCollect(ev *v1.Event) bool {
	involvedObject := ev.InvolvedObject

	kind := strings.ToLower(involvedObject.Kind)
	reasonsByKind, ok := c.collectedTypes[kind]
	if !ok {
		return false
	}

	_, shouldCollect := reasonsByKind[ev.Reason]

	return shouldCollect
}
