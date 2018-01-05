// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.
// +build kubeapiserver

package cluster

import (
	"errors"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/ericchiang/k8s/api/v1"
	"math"
	"strings"
	"time"
)

type kubernetesEventBundle struct {
	objName       string
	readableKey   string
	component     string
	events        []*v1.Event
	timeStamp     int64
	countByAction map[string]int
}

func newKubernetesEventBundler(objName string, compName string) *kubernetesEventBundle {
	return &kubernetesEventBundle{
		objName:       objName,
		events:        []*v1.Event{},
		component:     compName,
		countByAction: make(map[string]int),
	}
}

func (k *kubernetesEventBundle) addEvent(event *v1.Event) error {
	if *event.InvolvedObject.Name != k.objName {
		return fmt.Errorf("mismatching Object name: %s != %s", event.InvolvedObject.Name, k.objName)
	}
	k.events = append(k.events, event)
	k.countByAction[*event.Reason] += int(*event.Count)
	k.timeStamp = int64(math.Max(float64(k.timeStamp), float64(*event.LastTimestamp.Seconds)))
	k.readableKey = fmt.Sprintf("%s %s", *event.InvolvedObject.Kind, *event.InvolvedObject.Name)
	return nil
}

func (k *kubernetesEventBundle) formatEvents(hostname string, modified bool) (metrics.Event, error) {
	output := metrics.Event{
		Priority:       metrics.EventPriorityNormal,
		Host:           hostname,
		SourceTypeName: "kubernetes",
		EventType:      kubernetesAPIServerCheckName,
		Ts:             int64(k.timeStamp),
		AggregationKey: fmt.Sprintf("kubernetes_apiserver:%s", k.objName),
	}
	if len(k.events) == 0 {
		return output, errors.New("no event to export")
	}

	output.Title = fmt.Sprintf("Events from the %s",
		k.readableKey)

	if modified {
		output.Text = fmt.Sprintf("%s events emitted by the %s seen at %s", formatStringIntMap(k.countByAction), k.component, time.Unix(k.timeStamp, 0))
		return output, nil
	}
	output.Text = fmt.Sprintf("%s new events emitted by the %s seen at %s", formatStringIntMap(k.countByAction), k.component, time.Unix(k.timeStamp, 0))
	return output, nil
}

func formatStringIntMap(input map[string]int) string {
	var parts []string
	for k, v := range input {
		parts = append(parts, fmt.Sprintf("%d %s", v, k))
	}
	return strings.Join(parts, " ")
}
