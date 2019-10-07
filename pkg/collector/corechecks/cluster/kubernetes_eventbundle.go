// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.
// +build kubeapiserver

package cluster

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"math"
)

type kubernetesEventBundle struct {
	objUid        types.UID      // Unique object Identifier used as the Aggregation key
	namespace     string         // namespace of the bundle
	readableKey   string         // Formated key used in the Title in the events
	component     string         // Used to identify the Kubernetes component which generated the event
	events        []*v1.Event    // List of events in the bundle
	timeStamp     float64        // Used for the new events in the bundle to specify when they first occurred
	lastTimestamp float64        // Used for the modified events in the bundle to specify when they last occurred
	countByAction map[string]int // Map of count per action to aggregate several events from the same ObjUid in one event
	nodename      string         // Stores the nodename that should be used to submit the events
}

func newKubernetesEventBundler(objUid types.UID, compName string) *kubernetesEventBundle {
	return &kubernetesEventBundle{
		objUid:        objUid,
		component:     compName,
		countByAction: make(map[string]int),
	}
}

func (b *kubernetesEventBundle) addEvent(event *v1.Event) error {
	// As some fields are optional, we want to avoid evaluating empty values.
	if event == nil || event.InvolvedObject.Kind == "" {
		return errors.New("could not retrieve some parent attributes of the event")
	}
	if event.Reason == "" || event.Message == "" || event.InvolvedObject.Name == "" {
		return errors.New("could not retrieve some attributes of the event")
	}
	if event.InvolvedObject.UID != b.objUid {
		return fmt.Errorf("mismatching Object UIDs: %s != %s", event.InvolvedObject.UID, b.objUid)
	}

	b.events = append(b.events, event)
	b.namespace = event.InvolvedObject.Namespace

	// We do not process the events in chronological order necessarily.
	// We only care about the first time they occured, the last time and the count.
	b.timeStamp = float64(event.FirstTimestamp.Unix())
	b.lastTimestamp = math.Max(b.lastTimestamp, float64(event.LastTimestamp.Unix()))

	b.countByAction[fmt.Sprintf("**%s**: %s\n", event.Reason, event.Message)] += int(event.Count)

	switch event.InvolvedObject.Kind {
	case "Node":
		b.readableKey = fmt.Sprintf("%s %s", event.InvolvedObject.Kind, event.InvolvedObject.Name)
		b.nodename = event.Source.Host
	case "Pod":
		b.readableKey = fmt.Sprintf("%s %s/%s", event.InvolvedObject.Kind, event.InvolvedObject.Namespace, event.InvolvedObject.Name)
		b.nodename = event.Source.Host
	}

	return nil
}

func (b *kubernetesEventBundle) formatEvents(clusterName string) (metrics.Event, error) {
	if len(b.events) == 0 {
		return metrics.Event{}, errors.New("no event to export")
	}
	// Adding the clusterName to the nodename if present
	hostname := b.nodename
	if b.nodename != "" && clusterName != "" {
		hostname = hostname + "-" + clusterName
	}
	// If hostname was not defined, the aggregator will then set the local hostname
	output := metrics.Event{
		Title:          fmt.Sprintf("Events from the %s", b.readableKey),
		Priority:       metrics.EventPriorityNormal,
		Host:           hostname,
		SourceTypeName: "kubernetes",
		EventType:      kubernetesAPIServerCheckName,
		Ts:             int64(b.lastTimestamp),
		Tags:           []string{fmt.Sprintf("source_component:%s", b.component)},
		AggregationKey: fmt.Sprintf("kubernetes_apiserver:%s", b.objUid),
	}
	if b.namespace != "" {
		output.Tags = append(output.Tags, fmt.Sprintf("namespace:%s", b.namespace))
	}
	output.Text = "%%% \n" + fmt.Sprintf("%s \n _Events emitted by the %s seen at %s since %s_ \n", formatStringIntMap(b.countByAction), b.component, time.Unix(int64(b.lastTimestamp), 0), time.Unix(int64(b.timeStamp), 0)) + "\n %%%"
	return output, nil
}

func formatStringIntMap(input map[string]int) string {
	var parts []string
	for k, v := range input {
		parts = append(parts, fmt.Sprintf("%d %s", v, k))
	}
	return strings.Join(parts, " ")
}
