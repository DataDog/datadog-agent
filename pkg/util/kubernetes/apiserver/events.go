// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build kubeapiserver

package apiserver

//// Covered by test/integration/util/kube_apiserver/events_test.go

import (
	"context"
	"io"
	"strconv"
	"time"

	log "github.com/cihub/seelog"
	"github.com/ericchiang/k8s"
	"github.com/ericchiang/k8s/api/v1"
)

var eventReadTimeout = 100 * time.Millisecond

// LatestEvents retrieves all the cluster events happening after a given token.
// First slice is the new events, second slice the modified events.
// If the `since` parameter is empty, we query the apiserver's cache to avoid
// overloading it.
// https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.9/#watch-list-289
func (c *APIClient) LatestEvents(since string) ([]*v1.Event, []*v1.Event, string, error) {
	var addedEvents, modifiedEvents []*v1.Event
	latestResVersion := &since

	// If `since` is "" strconv.Atoi(*latestResVersion) below will panic as we evaluate the error.
	// One could chose to use "" instead of 0 to not query the API Server cache.
	// We decide to only rely on the cache as it avoids crawling everything from the API Server at once.
	log.Debugf("since value is %q", since)
	resVersionCached, err := strconv.Atoi(since)
	if err != nil {
		log.Errorf("The cached event token could not be parsed: %s, pulling events from the API server's cache", err)
	}
	var sinceOption k8s.Option

	if len(since) > 0 {
		sinceOption = k8s.ResourceVersion(since)
	} else {
		// Only retrieve what is in the apiserver cache.
		// Else we'll get one hour worth of events.
		sinceOption = k8s.ResourceVersion("0")
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	watcher, err := c.client.CoreV1().WatchEvents(ctx, "", sinceOption)
	if err != nil {
		return addedEvents, modifiedEvents, since, err
	}

	// Start a timer that will cancel the connection when the watcher is idle.
	// That allows `watcher.Next()` to return context.Canceled and break the reading loop.
	timeout := time.AfterFunc(eventReadTimeout, func() { cancel() })

	for {
		meta, event, err := watcher.Next()
		if err != nil {
			if err != context.Canceled && err != io.EOF {
				log.Debugf("Stopping event collection, got error: %s", err)
			} // else silently stop
			break
		}
		timeout.Reset(eventReadTimeout)
		if event == nil || event.Metadata == nil || event.Metadata.ResourceVersion == nil || event.Metadata.Uid == nil {
			log.Tracef("Skipping invalid event: %v", event)
			continue
		}

		resVersionMetadata, kubeEventErr := strconv.Atoi(*event.Metadata.ResourceVersion)
		if kubeEventErr != nil {
			log.Errorf("The Resource version associated with the event %s is not supported: %s", *event.Metadata.Uid, err.Error())
			continue
		}

		if resVersionMetadata > resVersionCached {
			latestResVersion = event.Metadata.ResourceVersion
			resVersionCached = resVersionMetadata
		}

		if meta == nil || meta.Type == nil {
			log.Debugf("skipping invalid event: %v", meta)
			continue
		}
		switch *meta.Type {
		case k8s.EventAdded:
			addedEvents = append(addedEvents, event)
		case k8s.EventModified:
			modifiedEvents = append(modifiedEvents, event)
		}
	}
	watcher.Close()
	return addedEvents, modifiedEvents, *latestResVersion, nil
}
