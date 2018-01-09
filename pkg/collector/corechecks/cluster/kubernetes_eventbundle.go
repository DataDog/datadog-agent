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
	objUid        string
	readableKey   string
	component     string
	events        []*v1.Event
	timeStamp     int64
	lastTimestamp int64
	countByAction map[string]int
}

func newKubernetesEventBundler(objUid string, compName string) *kubernetesEventBundle {
	return &kubernetesEventBundle{
		objUid:        objUid,
		events:        []*v1.Event{},
		component:     compName,
		countByAction: make(map[string]int),
	}
}

func (k *kubernetesEventBundle) addEvent(event *v1.Event) error {
	if *event.InvolvedObject.Uid != k.objUid {
		return fmt.Errorf("mismatching Object UIDs: %s != %s", event.InvolvedObject.Uid, k.objUid)
	}
	k.events = append(k.events, event)
	k.countByAction[fmt.Sprintf("**%s**: %s\n", *event.Reason, *event.Message)] += int(*event.Count)
	k.timeStamp = int64(math.Max(float64(k.timeStamp), float64(*event.Metadata.CreationTimestamp.Seconds)))
	k.lastTimestamp = int64(math.Max(float64(k.timeStamp), float64(*event.LastTimestamp.Seconds)))
	k.readableKey = fmt.Sprintf("%s %s", *event.InvolvedObject.Name, *event.InvolvedObject.Kind)
	return nil
}

func (k *kubernetesEventBundle) formatEvents(hostname string, modified bool) (metrics.Event, error) {
	output := metrics.Event{
		Priority:       metrics.EventPriorityNormal,
		Host:           hostname,
		SourceTypeName: "kubernetes",
		EventType:      kubernetesAPIServerCheckName,
		Ts:             int64(k.timeStamp),
		AggregationKey: fmt.Sprintf("kubernetes_apiserver:%s", k.objUid),
	}
	if len(k.events) == 0 {
		return output, errors.New("no event to export")
	}

	output.Title = fmt.Sprintf("Events from the %s",
		k.readableKey)

	if modified {
		output.Text = "%%% \n" + fmt.Sprintf("%s \n _Events emitted by the %s seen at %s_ \n", formatStringIntMap(k.countByAction), k.component, time.Unix(k.lastTimestamp, 0)) + "\n %%%"
		output.Ts = int64(k.lastTimestamp)
		return output, nil
	}
	output.Text = "%%% \n" + fmt.Sprintf("%s \n _New events emitted by the %s seen at %s_ \n", formatStringIntMap(k.countByAction), k.component, time.Unix(k.timeStamp, 0)) + "\n %%%"
	return output, nil
}

func formatStringIntMap(input map[string]int) string {
	var parts []string
	for k, v := range input {
		parts = append(parts, fmt.Sprintf("%d %s", v, k))
	}
	return strings.Join(parts, " ")
}
