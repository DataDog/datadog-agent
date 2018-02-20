// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.
// +build kubeapiserver

package cluster

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

)

type kubernetesEventBundle struct {
	objUid        types.UID         // Unique object Identifier used as the Aggregation key
	namespace     string         // namespace of the bundle
	readableKey   string         // Formated key used in the Title in the events
	component     string         // Used to identify the Kubernetes component which generated the event
	events        []*v1.Event    // List of events in the bundle
	timeStamp     float64        // Used for the new events in the bundle to specify when they first occurred
	lastTimestamp float64        // Used for the modified events in the bundle to specify when they last occurred
	countByAction map[string]int // Map of count per action to aggregate several events from the same ObjUid in one event
}

func newKubernetesEventBundler(objUid types.UID, compName string) *kubernetesEventBundle {
	return &kubernetesEventBundle{
		objUid:        objUid,
		component:     compName,
		countByAction: make(map[string]int),
	}
}

func (k *kubernetesEventBundle) addEvent(event *v1.Event) error {
	// As some fields are optional, we want to avoid evaluating empty values.
	if event == nil || event.InvolvedObject.Kind == "" {
		return errors.New("could not retrieve some parent attributes of the event")
	}
	if event.Reason == "" || event.Message == "" || event.InvolvedObject.Name == "" {
		return errors.New("could not retrieve some attributes of the event")
	}
	if event.InvolvedObject.UID != k.objUid {
		return fmt.Errorf("mismatching Object UIDs: %s != %s", event.InvolvedObject.UID, k.objUid)
	}

	k.events = append(k.events, event)
	k.namespace = event.InvolvedObject.Namespace
	k.timeStamp = math.Max(k.timeStamp, float64(event.CreationTimestamp.Second()))
	k.lastTimestamp = math.Max(k.timeStamp, float64(event.LastTimestamp.Second()))

	k.countByAction[fmt.Sprintf("**%s**: %s\n", event.Reason, event.Message)] += int(event.Count)
	k.readableKey = fmt.Sprintf("%s %s", event.InvolvedObject.Name, event.InvolvedObject.Kind)
	return nil
}

func (k *kubernetesEventBundle) formatEvents(hostname string, modified bool) (metrics.Event, error) {
	if len(k.events) == 0 {
		return metrics.Event{}, errors.New("no event to export")
	}
	output := metrics.Event{
		Title:          fmt.Sprintf("Events from the %s", k.readableKey),
		Priority:       metrics.EventPriorityNormal,
		Host:           hostname,
		SourceTypeName: "kubernetes",
		EventType:      kubernetesAPIServerCheckName,
		Ts:             int64(k.timeStamp),
		Tags:           []string{fmt.Sprintf("source_component:%s", k.component)},
		AggregationKey: fmt.Sprintf("kubernetes_apiserver:%s", k.objUid),
	}
	if k.namespace != "" {
		output.Tags = append(output.Tags, fmt.Sprintf("namespace:%s", k.namespace))
	}
	if modified {
		output.Text = "%%% \n" + fmt.Sprintf("%s \n _Events emitted by the %s seen at %s_ \n", formatStringIntMap(k.countByAction), k.component, time.Unix(int64(k.lastTimestamp), 0)) + "\n %%%"
		output.Ts = int64(k.lastTimestamp)
		return output, nil
	}
	output.Text = "%%% \n" + fmt.Sprintf("%s \n _New events emitted by the %s seen at %s_ \n", formatStringIntMap(k.countByAction), k.component, time.Unix(int64(k.timeStamp), 0)) + "\n %%%"
	return output, nil
}

func formatStringIntMap(input map[string]int) string {
	var parts []string
	for k, v := range input {
		parts = append(parts, fmt.Sprintf("%d %s", v, k))
	}
	return strings.Join(parts, " ")
}
