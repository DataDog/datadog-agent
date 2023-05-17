// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package helm

import (
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	coreMetrics "github.com/DataDog/datadog-agent/pkg/metrics"
)

const (
	eventTitle       = "Event on Helm release"
	helmStatusFailed = "failed"
)

type eventsManager struct {
	events      []coreMetrics.Event
	eventsMutex sync.Mutex
}

func (em *eventsManager) addEventForNewRelease(rel *release, tags []string) {
	event := eventForRelease(rel, textForAddEvent(rel), tags)
	em.storeEvent(event)
}

func (em *eventsManager) addEventForDeletedRelease(rel *release, tags []string) {
	event := eventForRelease(rel, textForDeleteEvent(rel), tags)
	em.storeEvent(event)
}

func (em *eventsManager) addEventForUpdatedRelease(old *release, updated *release, tags []string) {
	// nil Info should not happen, so let's ignore those at least for now.
	if (old.Info == nil || updated.Info == nil) || (old.Info.Status == updated.Info.Status) {
		return
	}

	event := eventForRelease(updated, textForChangedStatus(old.Info.Status, updated), tags)
	em.storeEvent(event)
}

func (em *eventsManager) sendEvents(sender aggregator.Sender) {
	em.eventsMutex.Lock()
	eventsToSend := em.events
	em.events = nil
	em.eventsMutex.Unlock()

	for _, event := range eventsToSend {
		sender.Event(event)
	}
}

func (em *eventsManager) storeEvent(event coreMetrics.Event) {
	em.eventsMutex.Lock()
	defer em.eventsMutex.Unlock()
	em.events = append(em.events, event)
}

func eventForRelease(rel *release, text string, tags []string) coreMetrics.Event {
	status := ""
	if rel.Info != nil {
		status = rel.Info.Status
	}

	return coreMetrics.Event{
		Title:          eventTitle,
		Text:           text,
		Ts:             time.Now().Unix(),
		Priority:       coreMetrics.EventPriorityNormal,
		SourceTypeName: checkName,
		EventType:      checkName,
		AggregationKey: fmt.Sprintf("helm_release:%s", rel.namespacedName()),
		Tags:           tags,
		AlertType:      alertType(status),
	}
}

func textForAddEvent(rel *release) string {
	status := ""
	if rel.Info != nil {
		status = rel.Info.Status
	}

	if rel.Version == 1 {
		return fmt.Sprintf("New Helm release %q has been deployed in %q namespace. Its status is %q.",
			rel.Name,
			rel.Namespace,
			status)
	}

	return fmt.Sprintf("Helm release %q in %q namespace upgraded to revision %d. Its status is %q.",
		rel.Name,
		rel.Namespace,
		rel.Version,
		status)
}

func textForDeleteEvent(rel *release) string {
	return fmt.Sprintf("Helm release %q in %q namespace has been deleted.", rel.Name, rel.Namespace)
}

func textForChangedStatus(previousRelStatus string, updatedRelease *release) string {
	return fmt.Sprintf("Helm release %q (revision %d) in %q namespace changed its status from %q to %q.",
		updatedRelease.Name,
		updatedRelease.Version,
		updatedRelease.Namespace,
		previousRelStatus,
		updatedRelease.Info.Status)
}

func alertType(releaseStatus string) coreMetrics.EventAlertType {
	if releaseStatus == helmStatusFailed {
		return coreMetrics.EventAlertTypeError
	}

	return coreMetrics.EventAlertTypeInfo
}
