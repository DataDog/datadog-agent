// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build kubeapiserver

package apiserver

//// Covered by test/integration/util/kube_apiserver/events_test.go

import (
	log "github.com/cihub/seelog"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"strconv"
	"time"
	"github.com/pkg/errors"
)

var eventReadTimeout = 100 * time.Millisecond

// LatestEvents retrieves all the cluster events happening after a given token.
// First slice is the new events, second slice the modified events.
// If the `since` parameter is empty, we query the apiserver's cache to avoid
// overloading it.
// https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.9/#watch-list-289
func (c *APIClient) LatestEvents(since string) ([]*v1.Event, []*v1.Event, string, error) {
	var addedEvents, modifiedEvents []*v1.Event

	// If `since` is "" strconv.Atoi(*latestResVersion) below will panic as we evaluate the error.
	// One could chose to use "" instead of 0 to not query the API Server cache.
	// We decide to only rely on the cache as it avoids crawling everything from the API Server at once.

	log.Tracef("since value is %q", since)

	resVersionCached, err := strconv.Atoi(since)
	if err != nil {
		log.Errorf("The cached event token could not be parsed: %s, pulling events from the API server's cache", err)
	}

	eventWatcher, errs := c.client.Events("").Watch(metav1.ListOptions{Watch: true,ResourceVersion: since})
	if errs != nil {
		log.Debugf("error getting watcher")
	}
	watcherTimeout := time.NewTimer(eventReadTimeout)
	for {
		select {
		case rcvdEv := <-eventWatcher.ResultChan():
			if rcvdEv.Type == watch.Error {
				errEvent, ok := rcvdEv.Object.(*metav1.Status)
				if !ok {
					return nil,nil,"0",errors.New("could not parse status of error event from the API Server.") // TODO custom error
				}
				if errEvent.Reason == "Expired"{
					// Known issue with ETCD https://github.com/kubernetes/kubernetes/issues/45506
					// Once we have started the event watcher, we do not expect the resversion to be "0"
					// Except when initializing. Forcing to 0 so we can keep on watching without hitting the cache.
					log.Tracef("Resversion expired: %s", errEvent.Message)
					resVersionCached = 0
				}
			}

			currEvent, ok := rcvdEv.Object.(*v1.Event)
			if !ok {
				log.Debugf("Unexpected format for the evaluated event. Skipping.")
				continue
			}

			evResVer, err := strconv.Atoi(currEvent.ResourceVersion)
			if err != nil {
				// Could not convert the Resversion. Returning.
				return addedEvents, modifiedEvents, "0", err
			}
			if evResVer > resVersionCached {
				resVersionCached = evResVer
			}

			if rcvdEv.Type == watch.Added {
				addedEvents = append(addedEvents, currEvent)
				resVersionCached = evResVer
			}
			if rcvdEv.Type == watch.Modified {
				modifiedEvents = append(modifiedEvents, currEvent)
				resVersionCached = evResVer
			}
			watcherTimeout.Reset(eventReadTimeout)

		case <-watcherTimeout.C:
			// No more events to read or the crawl lasted more than `eventReadTimeout`.
			// Gracefully closing the watcher returning what was processed.
			eventWatcher.Stop()
			return addedEvents, modifiedEvents, strconv.Itoa(resVersionCached), nil
		}
	}
}
