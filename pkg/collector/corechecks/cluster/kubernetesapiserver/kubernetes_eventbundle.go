// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

//go:build kubeapiserver

package kubernetesapiserver

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/pkg/metrics"
)

type kubernetesEventBundle struct {
	involvedObject v1.ObjectReference     // Parent object for this event bundle
	component      string                 // Used to identify the Kubernetes component which generated the event
	timeStamp      float64                // Used for the new events in the bundle to specify when they first occurred
	lastTimestamp  float64                // Used for the modified events in the bundle to specify when they last occurred
	countByAction  map[string]int         // Map of count per action to aggregate several events from the same ObjUid in one event
	alertType      metrics.EventAlertType // The Datadog event type
	hostInfo       eventHostInfo          // Host information extracted from the event, where applicable
}

func newKubernetesEventBundler(clusterName string, event *v1.Event) *kubernetesEventBundle {
	return &kubernetesEventBundle{
		involvedObject: event.InvolvedObject,
		component:      event.Source.Component,
		countByAction:  make(map[string]int),
		alertType:      getDDAlertType(event.Type),
		hostInfo:       getEventHostInfo(clusterName, event),
	}
}

func (b *kubernetesEventBundle) addEvent(event *v1.Event) error {
	if event.InvolvedObject.UID != b.involvedObject.UID {
		return fmt.Errorf("mismatching Object UIDs: %s != %s", event.InvolvedObject.UID, b.involvedObject.UID)
	}

	// We do not process the events in chronological order necessarily.
	// We only care about the first time they occurred, the last time and the count.
	b.timeStamp = float64(event.FirstTimestamp.Unix())
	b.lastTimestamp = math.Max(b.lastTimestamp, float64(event.LastTimestamp.Unix()))

	b.countByAction[fmt.Sprintf("**%s**: %s\n", event.Reason, event.Message)] += int(event.Count)

	return nil
}

func (b *kubernetesEventBundle) formatEvents() (metrics.Event, error) {
	if len(b.countByAction) == 0 {
		return metrics.Event{}, errors.New("no event to export")
	}

	readableKey := buildReadableKey(b.involvedObject)
	tags := getInvolvedObjectTags(b.involvedObject)
	tags = append(tags, fmt.Sprintf("source_component:%s", b.component))

	if b.hostInfo.providerID != "" {
		tags = append(tags, fmt.Sprintf("host_provider_id:%s", b.hostInfo.providerID))
	}

	// If hostname was not defined, the aggregator will then set the local hostname
	output := metrics.Event{
		Title:          fmt.Sprintf("Events from the %s", readableKey),
		Priority:       metrics.EventPriorityNormal,
		Host:           b.hostInfo.hostname,
		SourceTypeName: "kubernetes",
		EventType:      kubernetesAPIServerCheckName,
		Ts:             int64(b.lastTimestamp),
		Tags:           tags,
		AggregationKey: fmt.Sprintf("kubernetes_apiserver:%s", b.involvedObject.UID),
		AlertType:      b.alertType,
		Text:           b.formatEventText(),
	}

	return output, nil
}

func (b *kubernetesEventBundle) formatEventText() string {
	eventText := fmt.Sprintf(
		"%%%%%% \n%s \n _Events emitted by the %s seen at %s since %s_ \n\n %%%%%%",
		formatStringIntMap(b.countByAction),
		b.component,
		time.Unix(int64(b.lastTimestamp), 0),
		time.Unix(int64(b.timeStamp), 0),
	)

	// Escape the ~ character to not strike out the text
	eventText = strings.ReplaceAll(eventText, "~", "\\~")

	return eventText
}

func formatStringIntMap(input map[string]int) string {
	parts := make([]string, 0, len(input))
	keys := make([]string, 0, len(input))

	for k := range input {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	for _, k := range keys {
		v := input[k]
		parts = append(parts, fmt.Sprintf("%d %s", v, k))
	}

	return strings.Join(parts, " ")
}
