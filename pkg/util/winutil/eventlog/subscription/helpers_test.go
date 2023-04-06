// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.
//go:build windows
// +build windows

package evtsubscribe

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/test"

	"github.com/stretchr/testify/assert"
)

func ReadNumEventsWithNotify(t testing.TB, ti eventlog_test.APITester, sub PullSubscription, numEvents uint) ([]*evtapi.EventRecord, error) {
	eventRecords := make([]*evtapi.EventRecord, 0)

	count := uint(0)
eventLoop:
	for {
		select {
		case _, ok := <-sub.EventsAvailable():
			if !ok {
				break eventLoop
			}
			for {
				events, err := sub.GetEvents()
				if !assert.NoError(t, err, "GetEvents should not return an error") {
					return nil, fmt.Errorf("GetEvents returned error: %v", err)
				}
				if count == numEvents {
					if !assert.Nil(t, events, "events should be nil when count is reached") {
						return nil, fmt.Errorf("events should be nil when count is reached")
					}
				} else {
					if !assert.NotNil(t, events, "events should not be nil if count is not reached %v/%v", count, numEvents) {
						return nil, fmt.Errorf("events should not be nil")
					}
				}
				if events != nil {
					eventRecords = append(eventRecords, events...)
					count += uint(len(events))
				}
				if count >= numEvents {
					break eventLoop
				}
			}
		}
	}

	for _, eventRecord := range eventRecords {
		if !assert.NotEqual(t, evtapi.EventRecordHandle(0), eventRecord.EventRecordHandle, "EventRecordHandle should not be NULL") {
			return nil, fmt.Errorf("EventRecordHandle should not be NULL")
		}
	}

	return eventRecords, nil
}

func ReadNumEvents(t testing.TB, ti eventlog_test.APITester, sub PullSubscription, numEvents uint) ([]*evtapi.EventRecord, error) {
	eventRecords := make([]*evtapi.EventRecord, 0)

	count := uint(0)
	for {
		events, err := sub.GetEvents()
		if !assert.NoError(t, err, "GetEvents should not return an error") {
			return nil, fmt.Errorf("GetEvents returned error: %v", err)
		}
		if count == numEvents {
			if !assert.Nil(t, events, "events should be nil when count is reached") {
				return nil, fmt.Errorf("events should be nil when count is reached")
			}
		} else {
			if !assert.NotNil(t, events, "events should not be nil if count is not reached %v/%v", count, numEvents) {
				return nil, fmt.Errorf("events should not be nil")
			}
		}
		if events != nil {
			eventRecords = append(eventRecords, events...)
			count += uint(len(events))
		}
		if count >= numEvents {
			break
		}
	}

	for _, eventRecord := range eventRecords {
		if !assert.NotEqual(t, evtapi.EventRecordHandle(0), eventRecord.EventRecordHandle, "EventRecordHandle should not be NULL") {
			return nil, fmt.Errorf("EventRecordHandle should not be NULL")
		}
	}

	return eventRecords, nil
}
