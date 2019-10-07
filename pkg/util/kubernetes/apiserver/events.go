// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build kubeapiserver

package apiserver

//// Covered by test/integration/util/kube_apiserver/apiserver_test.go

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"strconv"
	"time"
)

// RunEventCollection will return the most recent events emitted by the apiserver.
func (c *APIClient) RunEventCollection(srv *string, st *time.Time, eventReadTimeout *int64, eventCardinalityLimit int64, filter string) ([]*v1.Event, error) {
	var added []*v1.Event
	nrv := srv
	// list if latestResVer is "" or if lastListTS is > maxResync
	if *srv == "" || time.Now().Second()-st.Second() > 300 {
		evList, err := c.Cl.CoreV1().Events(metav1.NamespaceAll).List(metav1.ListOptions{
			TimeoutSeconds:       eventReadTimeout,
			Limit:                eventCardinalityLimit,
			IncludeUninitialized: false,
			FieldSelector:        filter,
		})
		if err != nil {
			// TODO revisit error handling here
			return nil, err
		}
		for _, e := range evList.Items {
			// List call returns a different type than the Watch call.
			added = append(added, &e)
		}
		if len(evList.Items) == int(eventCardinalityLimit) {
			log.Infof("Limitted collection to the %d most recent events", eventCardinalityLimit)
		}
		*st = time.Now()
		*srv = evList.ResourceVersion
	}

	// Start watcher with the most up to date RV
	evWatcher, err := c.Cl.CoreV1().Events(metav1.NamespaceAll).Watch(metav1.ListOptions{
		Watch:                true,
		ResourceVersion:      *nrv,
		Limit:                eventCardinalityLimit,
		IncludeUninitialized: false,
		FieldSelector:        filter,
	})
	if err != nil {
		return added, err
	}
	defer evWatcher.Stop()
	log.Debugf("Starting to watch from %s", *nrv)
	// watch during 2 * timeout maximum and store where we are at.
	timeoutParse := time.NewTimer(time.Duration(*eventReadTimeout*2) * time.Second)
	for {
		select {
		case rcv, ok := <-evWatcher.ResultChan():
			if !ok {
				log.Error("Unexpected watch close")
				return added, fmt.Errorf("Unexpected watch close")
			}
			if rcv.Type == watch.Error {
				status, ok := rcv.Object.(*metav1.Status)
				if !ok {
					// TODO revisit error handling here.
					return added, fmt.Errorf("Could not get status of ev ?")
				}
				if status.Reason == "Expired" {
					log.Debugf("RV is too old, using list and diffing to keep events more recent than the stored RV. \n")
					evList, err := c.Cl.CoreV1().Events("").List(metav1.ListOptions{
						//TimeoutSeconds: eventReadTimeout,
						Limit:                eventCardinalityLimit,
						IncludeUninitialized: false,
						FieldSelector:        filter,
					})
					if err != nil {
						return added, err
					}
					log.Infof("Listed %d events", len(evList.Items))
					*st = time.Now()
					i, _ := strconv.Atoi(*nrv)
					ev := diffEvents(i, evList.Items)
					*nrv = evList.ResourceVersion
					// List, Do the diff, return delta ev, return newest RV from the List
					// will return here
					return ev, nil
				}
				// if other error, we return the err and RV "" to relist in the next run!, could be a network blip/.

			}
			ev, ok := rcv.Object.(*v1.Event)
			if !ok {
				// Could not cast the ev, might as well drop this ev, and continue.
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

			i, err := strconv.Atoi(*nrv)
			if err != nil {
				log.Errorf("Could not cast %s into an integer: %s", *nrv, err.Error())
				continue
			}
			if evResVer > i {
				// Events from the watch are not ordered necessarily, let's keep track of the newest RV.
				*nrv = ev.ResourceVersion
			}

		case <-timeoutParse.C:
			log.Debugf("Collected %d events, will resume watching from RV %s in 10 seconds", len(added), *nrv)
			// No more events to read or the watch lasted more than `eventReadTimeout`.
			// so return what was processed.
			return added, nil
		}
	}
}

func diffEvents(latestStoredRV int, fullList []v1.Event) []*v1.Event {
	var diffEvents []*v1.Event
	for _, ev := range fullList {
		i, _ := strconv.Atoi(ev.ResourceVersion)
		if i > latestStoredRV {
			diffEvents = append(diffEvents, &ev)
		}
	}
	log.Debugf("returning %d events that we have not collected", len(diffEvents))
	return diffEvents
}
