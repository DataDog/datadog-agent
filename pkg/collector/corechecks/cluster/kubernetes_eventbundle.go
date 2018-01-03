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
	nodeUid       string
	events        []*v1.Event
	firstTime     int64
	lastTime      int64
	countByAction map[string]int
}

func newKubernetesEventBundler(nodeUid string) *kubernetesEventBundle {
	return &kubernetesEventBundle{
		nodeUid:       nodeUid,
		events:        []*v1.Event{},
		countByAction: make(map[string]int),
	}
}

func (k *kubernetesEventBundle) addEvent(event *v1.Event) error {
	if *event.InvolvedObject.Uid != k.nodeUid {
		return fmt.Errorf("mismatching Node Id name: %s != %s", event.InvolvedObject.Uid, k.nodeUid)
	}
	k.events = append(k.events, event)
	k.countByAction[*event.Reason] += int(*event.Count)
	k.firstTime = *event.FirstTimestamp.Seconds
	k.lastTime = int64(math.Max(float64(k.lastTime), float64(*event.LastTimestamp.Seconds)))
	return nil
}

func (k *kubernetesEventBundle) formatEvents(hostname string, modified bool) (metrics.Event, error) {
	output := metrics.Event{
		Priority:       metrics.EventPriorityNormal,
		Host:           hostname,
		SourceTypeName: kubernetesAPIServerCheck,
		EventType:      kubernetesAPIServerCheck,
		Ts:             int64(k.lastTime),
		AggregationKey: fmt.Sprintf("kubernetes_apiserver:%s", k.nodeUid),
	}
	if len(k.events) == 0 {
		return output, errors.New("no event to export")
	}
	output.Title = fmt.Sprintf("Events of %s on %s",
		k.nodeUid,
		hostname)

	//if k.lastTime != k.firstTime {
	//	// Modified events
	//	output.Text = fmt.Sprintf("%s events seen at %s", formatStringIntMap(k.countByAction), time.Unix(k.lastTime,0))
	//	return output, nil
	//}
	output.Text = fmt.Sprintf("%s new events seen at %s", formatStringIntMap(k.countByAction), time.Unix(k.lastTime, 0))
	return output, nil
}

func formatStringIntMap(input map[string]int) string {
	var parts []string
	for k, v := range input {
		parts = append(parts, fmt.Sprintf("%d %s", v, k))
	}
	return strings.Join(parts, " ")
}
