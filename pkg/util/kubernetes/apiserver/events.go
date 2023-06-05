// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

//go:build kubeapiserver

package apiserver

//// Covered by test/integration/util/kube_apiserver/apiserver_test.go

import (
	"context"
	"fmt"
	"strconv"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// RunEventCollection will return the most recent events emitted by the apiserver.
func (c *APIClient) RunEventCollection(resVer string, lastListTime time.Time, eventReadTimeout int64, eventCardinalityLimit int64, resync int64, filter string) ([]*v1.Event, string, time.Time, error) {
	var added []*v1.Event
	syncTimeout := time.Duration(resync) * time.Second
	// list if latestResVer is "" or if lastListTS is > syncTimeout
	diffTime := time.Now().Sub(lastListTime)
	if resVer == "" || diffTime > syncTimeout {
		log.Debugf("Return listForEventResync diffTime: %d/%d", diffTime, syncTimeout)
		listed, lastResVer, lastTime, err := c.listForEventResync(eventReadTimeout, eventCardinalityLimit, filter)
		if err != nil {
			return nil, "", time.Now(), err
		}
		resVerInt, errConv := strconv.Atoi(resVer)
		if errConv != nil {
			// resver can be "" if we need to resync
			resVerInt = 0
		}
		return diffEvents(resVerInt, listed), lastResVer, lastTime, nil
	}
	// Start watcher with the most up to date RV
	evWatcher, err := c.Cl.CoreV1().Events(metav1.NamespaceAll).Watch(context.TODO(), metav1.ListOptions{
		Watch:           true,
		ResourceVersion: resVer,
		Limit:           eventCardinalityLimit,
		FieldSelector:   filter,
	})
	if err != nil {
		return added, resVer, lastListTime, err
	}

	defer evWatcher.Stop()
	log.Debugf("Starting to watch from %s", resVer)
	// watch during 2 * timeout maximum and store where we are at.
	timeoutParse := time.NewTimer(time.Duration(eventReadTimeout) * time.Second)
	for {
		select {
		case rcv, ok := <-evWatcher.ResultChan():
			if !ok {
				return added, resVer, lastListTime, fmt.Errorf("Unexpected watch close")
			}
			if rcv.Type == watch.Error {
				status, ok := rcv.Object.(*metav1.Status)
				if !ok {
					return added, resVer, lastListTime, fmt.Errorf("Could not unmarshall the status of the event")
				}
				switch status.Reason {
				// Using a switch as there are a lot of different types and we might want to explore adapting the behaviour for certain ones in the future.
				case "Expired":
					log.Debugf("Resource Version is too old, listing all events and collecting only the new ones")
					evList, resVer, lastListTime, err := c.listForEventResync(eventReadTimeout, eventCardinalityLimit, filter)
					if err != nil {
						return added, resVer, lastListTime, err
					}
					i, err := strconv.Atoi(resVer)
					if err != nil {
						log.Errorf("Error converting the stored Resource Version: %s", err.Error())
						continue
					}
					return diffEvents(i, evList), resVer, lastListTime, nil
				default:
					// see the different types: k8s.io/apimachinery/pkg/apis/meta/v1/types.go
					return added, resVer, lastListTime, fmt.Errorf("received an unexpected status while collecting the events: %s", status.Reason)
				}
			}

			if rcv.Type == watch.Deleted {
				// The events informer sends the state of an object immediately before deletion.
				// We're not interested in re-processing these events because they should be processed already when they were added.
				// This happens when an event reaches the events TTL, an apiserver config (default 1 hour).
				// Ignoring this type of informer events will prevent from sending duplicated datadog events.
				continue
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
				return added, resVer, lastListTime, err
			}
			added = append(added, ev)
			i, err := strconv.Atoi(resVer)
			if err != nil {
				log.Errorf("Could not cast %s into an integer: %s", resVer, err.Error())
				continue
			}
			if evResVer > i {
				// Events from the watch are not ordered necessarily, let's keep track of the newest RV.
				resVer = ev.ResourceVersion
			}

		case <-timeoutParse.C:
			log.Debugf("Collected %d events, will resume watching from resource version %s", len(added), resVer)
			// No more events to read or the watch lasted more than `eventReadTimeout`.
			// so return what was processed.
			return added, resVer, lastListTime, nil
		}
	}
}

func diffEvents(latestStoredRV int, fullList []*v1.Event) []*v1.Event {
	var diffEvents []*v1.Event
	for _, ev := range fullList {
		erv, err := strconv.Atoi(ev.ResourceVersion)
		if err != nil {
			log.Errorf("Could not parse resource version of an event, will skip: %s", err)
			continue
		}
		if erv > latestStoredRV {
			diffEvents = append(diffEvents, ev)
		}
	}
	log.Debugf("Returning %d events that we have not collected", len(diffEvents))
	return diffEvents
}

func (c *APIClient) listForEventResync(eventReadTimeout int64, eventCardinalityLimit int64, filter string) (added []*v1.Event, resVer string, lastListTime time.Time, err error) {
	evList, err := c.Cl.CoreV1().Events(metav1.NamespaceAll).List(context.TODO(), metav1.ListOptions{
		TimeoutSeconds: &eventReadTimeout,
		Limit:          eventCardinalityLimit,
		FieldSelector:  filter,
	})
	if err != nil {
		log.Errorf("Error Listing events: %s", err.Error())
		return nil, resVer, lastListTime, err
	}
	for id := range evList.Items {
		// List call returns a different type than the Watch call.
		added = append(added, &evList.Items[id])
	}
	return added, evList.ResourceVersion, time.Now(), nil
}
