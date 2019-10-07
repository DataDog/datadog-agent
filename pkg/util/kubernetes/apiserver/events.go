// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build kubeapiserver

package apiserver

//// Covered by test/integration/util/kube_apiserver/apiserver_test.go

import (
	"fmt"
	"strconv"
	"time"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// RunEventCollection will return the most recent events emitted by the apiserver.
func (c *APIClient) RunEventCollection(srv *string, st *time.Time, eventReadTimeout *int64, eventCardinalityLimit int64, filter string) ([]*v1.Event, error) {
	var added []*v1.Event
	// list if latestResVer is "" or if lastListTS is > maxResync
	if *srv == "" || time.Now().Second()-st.Second() > 300 {
		evList, err := c.Cl.CoreV1().Events(metav1.NamespaceAll).List(metav1.ListOptions{
			TimeoutSeconds:       eventReadTimeout,
			Limit:                eventCardinalityLimit,
			IncludeUninitialized: false,
			FieldSelector:        filter,
		})
		if err != nil {
			log.Errorf("Error Listing events: %s", err.Error())
			return nil, err
		}
		for _, e := range evList.Items {
			// List call returns a different type than the Watch call.
			added = append(added, &e)
		}
		*st = time.Now()
		*srv = evList.ResourceVersion
		return added, nil
	}
	// Start watcher with the most up to date RV
	evWatcher, err := c.Cl.CoreV1().Events(metav1.NamespaceAll).Watch(metav1.ListOptions{
		Watch:                true,
		ResourceVersion:      *srv,
		Limit:                eventCardinalityLimit,
		IncludeUninitialized: false,
		FieldSelector:        filter,
	})
	if err != nil {
		return added, err
	}

	defer evWatcher.Stop()
	log.Debugf("Starting to watch from %s", *srv)
	// watch during 2 * timeout maximum and store where we are at.
	timeoutParse := time.NewTimer(time.Duration(*eventReadTimeout*2) * time.Second)
	for {
		select {
		case rcv, ok := <-evWatcher.ResultChan():
			if !ok {
				return added, fmt.Errorf("Unexpected watch close")
			}
			if rcv.Type == watch.Error {
				status, ok := rcv.Object.(*metav1.Status)
				if !ok {
					return added, fmt.Errorf("Could not unmarshall the status of the event")
				}
				switch status.Reason {
				// Using a switch as there are a lot of different types and we might want to explore adapting the behaviour for certain ones in the future.
				case "Expired":
					log.Debugf("Resource Version is too old, listing all events and collecting only the new ones")
					evList, err := c.Cl.CoreV1().Events(metav1.NamespaceAll).List(metav1.ListOptions{
						TimeoutSeconds:       eventReadTimeout,
						Limit:                eventCardinalityLimit,
						IncludeUninitialized: false,
						FieldSelector:        filter,
					})
					if err != nil {
						return added, err
					}
					*st = time.Now()
					i, err := strconv.Atoi(*srv)
					if err != nil {
						log.Errorf("Error converting the stored Resource Version: %s", err.Error())
						continue
					}
					ev := diffEvents(i, evList.Items)
					*srv = evList.ResourceVersion
					return ev, nil
				default:
					// see the different types: k8s.io/apimachinery/pkg/apis/meta/v1/types.go
					return added, fmt.Errorf("received an unexpected status while collecting the events: %s", status.Reason)
				}
			}

			ev, ok := rcv.Object.(*v1.Event)
			if !ok {
				// Could not cast the ev, might as well drop this event, and continue.
				log.Errorf("The event object for %v cannot be safely converted, skipping it.", rcv.Object)
				continue
			}
			evResVer, err := strconv.Atoi(ev.ResourceVersion)
			if err != nil {
				// Could not convert the Resversion. Returning.
				// should not be happening, it means the object is not correctly formatted in etcd.
				return added, err
			}
			added = append(added, ev)

			i, err := strconv.Atoi(*srv)
			if err != nil {
				log.Errorf("Could not cast %s into an integer: %s", *srv, err.Error())
				continue
			}
			if evResVer > i {
				// Events from the watch are not ordered necessarily, let's keep track of the newest RV.
				*srv = ev.ResourceVersion
			}

		case <-timeoutParse.C:
			log.Debugf("Collected %d events, will resume watching from RV %s", len(added), *srv)
			// No more events to read or the watch lasted more than `eventReadTimeout`.
			// so return what was processed.
			return added, nil
		}
	}
}

func diffEvents(latestStoredRV int, fullList []v1.Event) []*v1.Event {
	var diffEvents []*v1.Event
	for _, ev := range fullList {
		erv, _ := strconv.Atoi(ev.ResourceVersion)
		if erv > latestStoredRV {
			diffEvents = append(diffEvents, &ev)
		}
	}
	log.Debugf("Returning %d events that we have not collected", len(diffEvents))
	return diffEvents
}
