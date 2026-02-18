// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows

package evtsubscribe

import (
	"errors"
	"fmt"
	"testing"

	evtapi "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"
	evtbookmark "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/bookmark"
	eventlog_test "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/test"

	"github.com/stretchr/testify/assert"
)

func ReadNumEvents(t testing.TB, _ eventlog_test.APITester, sub PullSubscription, numEvents uint) ([]*evtapi.EventRecord, error) {
	eventRecords := make([]*evtapi.EventRecord, 0)

	var err error
	count := uint(0)
	for {
		events, ok := <-sub.GetEvents()
		if !ok {
			err = sub.Error()
		}
		if !assert.NoError(t, err, "GetEvents should not return an error") {
			return nil, fmt.Errorf("GetEvents returned error: %w", err)
		}
		if count == numEvents {
			if !assert.Nil(t, events, "events should be nil when count is reached") {
				return nil, errors.New("events should be nil when count is reached")
			}
		} else {
			if !assert.NotNil(t, events, "events should not be nil if count is not reached %v/%v", count, numEvents) {
				return nil, errors.New("events should not be nil")
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
			return nil, errors.New("EventRecordHandle should not be NULL")
		}
	}

	return eventRecords, nil
}

// bookmarkXMLFromEvent creates a bookmark XML from an event record
func bookmarkXMLFromEvent(api evtapi.API, event *evtapi.EventRecord) (string, error) {
	bookmark, err := evtbookmark.New(evtbookmark.WithWindowsEventLogAPI(api))
	if err != nil {
		return "", err
	}
	defer bookmark.Close()
	err = bookmark.Update(event.EventRecordHandle)
	if err != nil {
		return "", err
	}
	return bookmark.Render()
}
