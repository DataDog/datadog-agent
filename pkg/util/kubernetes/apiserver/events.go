// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build kubeapiserver

package apiserver

//// Covered by test/integration/util/kube_apiserver/events_test.go

import (
	"fmt"
	"reflect"
	"strconv"
	"time"

	"github.com/pkg/errors"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	eventReadTimeout = 100 * time.Millisecond
	expectedType     = reflect.TypeOf(v1.Event{})
)

// LatestEvents retrieves all the cluster events happening after a given token.
// First slice is the new events, second slice the modified events.
// If the `since` parameter is empty, we query the apiserver's cache to avoid
// overloading it.
// https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.9/#watch-list-289
func (c *APIClient) LatestEvents(since string) ([]*v1.Event, []*v1.Event, string, error) {
	var added, modified []*v1.Event

	// If `since` is "" strconv.Atoi(*latestResVersion) below will panic as we evaluate the error.
	// One could chose to use "" instead of 0 to not query the API Server cache.
	// We decide to only rely on the cache as it avoids crawling everything from the API Server at once.
	resVersionInt, err := strconv.Atoi(since)
	if err != nil {
		log.Errorf("The cached resourceVersion token %s, could not be parsed with: %s", since, err)
		since = "0"
	}

	log.Tracef("Starting watch of %v with resourceVersion %s", expectedType, since)

	eventWatcher, err := c.Cl.CoreV1().Events(metav1.NamespaceAll).Watch(metav1.ListOptions{Watch: true, ResourceVersion: since})
	if err != nil {
		return nil, nil, "0", fmt.Errorf("Failed to watch %v: %v", expectedType, err)
	}
	defer eventWatcher.Stop()

	watcherTimeout := time.NewTimer(eventReadTimeout)
	for {
		select {
		case rcvdEv, ok := <-eventWatcher.ResultChan():
			if !ok {
				log.Debugf("Unexpected watch close")
				return added, modified, strconv.Itoa(resVersionInt), nil
			}
			if rcvdEv.Type == watch.Error {
				status, ok := rcvdEv.Object.(*metav1.Status)
				if !ok {
					return nil, nil, "0", errors.New("could not parse status of error event from the API Server.") // TODO custom error
				}
				if status.Reason == "Expired" {
					// Known issue with ETCD https://github.com/kubernetes/kubernetes/issues/45506
					// Once we have started the event watcher, we do not expect the resversion to be "0"
					// Except when initializing. Forcing to 0 so we can keep on watching without hitting the cache.
					log.Tracef("Resversion expired: %s", status.Message)
					return added, modified, "0", nil
				}
				// We continue here to avoid casting into a *v1.Event.
				// In this case, the event is of type *metav1.Status.
				log.Debugf("Unexpected watch error: %s", status.Message)
				continue
			}

			currEvent, ok := rcvdEv.Object.(*v1.Event)
			if !ok {
				log.Debugf("The event object cannot be safely converted to %v: %v", expectedType, rcvdEv.Object)
				continue
			}

			evResVer, err := strconv.Atoi(currEvent.ResourceVersion)
			if err != nil {
				// Could not convert the Resversion. Returning.
				return added, modified, "0", err
			}
			if evResVer > resVersionInt {
				resVersionInt = evResVer
			}

			if rcvdEv.Type == watch.Added {
				added = append(added, currEvent)
				resVersionInt = evResVer
			}
			if rcvdEv.Type == watch.Modified {
				modified = append(modified, currEvent)
				resVersionInt = evResVer
			}
			watcherTimeout.Reset(eventReadTimeout)

		case <-watcherTimeout.C:
			// No more events to read or the watch lasted more than `eventReadTimeout`.
			// so return what was processed.
			return added, modified, strconv.Itoa(resVersionInt), nil
		}
	}
}
