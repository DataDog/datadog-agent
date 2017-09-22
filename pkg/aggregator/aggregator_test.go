// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package aggregator

import (
	// stdlib
	"testing"

	// 3p
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

var checkID1 check.ID = "1"
var checkID2 check.ID = "2"

func TestRegisterCheckSampler(t *testing.T) {
	resetAggregator()

	agg := InitAggregator(nil, "")
	err := agg.registerSender(checkID1)
	assert.Nil(t, err)
	assert.Len(t, aggregatorInstance.checkSamplers, 1)

	err = agg.registerSender(checkID2)
	assert.Nil(t, err)
	assert.Len(t, aggregatorInstance.checkSamplers, 2)

	// Already registered sender => error
	err = agg.registerSender(checkID2)
	assert.NotNil(t, err)
}

func TestDeregisterCheckSampler(t *testing.T) {
	resetAggregator()

	agg := InitAggregator(nil, "")
	agg.registerSender(checkID1)
	agg.registerSender(checkID2)
	assert.Len(t, aggregatorInstance.checkSamplers, 2)

	agg.deregisterSender(checkID1)
	require.Len(t, aggregatorInstance.checkSamplers, 1)
	_, ok := agg.checkSamplers[checkID1]
	assert.False(t, ok)
	_, ok = agg.checkSamplers[checkID2]
	assert.True(t, ok)
}

func TestAddServiceCheckDefaultValues(t *testing.T) {
	resetAggregator()
	agg := InitAggregator(nil, "resolved-hostname")

	agg.addServiceCheck(metrics.ServiceCheck{
		// leave Host and Ts fields blank
		CheckName: "my_service.can_connect",
		Status:    metrics.ServiceCheckOK,
		Tags:      []string{"bar", "foo", "bar"},
		Message:   "message",
	})
	agg.addServiceCheck(metrics.ServiceCheck{
		CheckName: "my_service.can_connect",
		Status:    metrics.ServiceCheckOK,
		Host:      "my-hostname",
		Tags:      []string{"foo", "foo", "bar"},
		Ts:        12345,
		Message:   "message",
	})

	require.Len(t, agg.serviceChecks, 2)
	assert.Equal(t, "resolved-hostname", agg.serviceChecks[0].Host)
	assert.Equal(t, []string{"bar", "foo"}, agg.serviceChecks[0].Tags)
	assert.NotZero(t, agg.serviceChecks[0].Ts) // should be set to the current time, let's just check that it's not 0
	assert.Equal(t, "my-hostname", agg.serviceChecks[1].Host)
	assert.Equal(t, []string{"foo", "bar"}, agg.serviceChecks[1].Tags)
	assert.Equal(t, int64(12345), agg.serviceChecks[1].Ts)
}

func TestAddEventDefaultValues(t *testing.T) {
	resetAggregator()
	agg := InitAggregator(nil, "resolved-hostname")

	agg.addEvent(metrics.Event{
		// only populate required fields
		Title: "An event occurred",
		Text:  "Event description",
	})
	agg.addEvent(metrics.Event{
		// populate all fields
		Title:          "Another event occurred",
		Text:           "Other event description",
		Ts:             12345,
		Priority:       metrics.EventPriorityNormal,
		Host:           "my-hostname",
		Tags:           []string{"foo", "bar", "foo"},
		AlertType:      metrics.EventAlertTypeError,
		AggregationKey: "my_agg_key",
		SourceTypeName: "custom_source_type",
	})

	require.Len(t, agg.events, 2)
	// Default values are set on Host and Ts only
	event1 := agg.events[0]
	assert.Equal(t, "An event occurred", event1.Title)
	assert.Equal(t, "resolved-hostname", event1.Host)
	assert.NotZero(t, event1.Ts) // should be set to the current time, let's just check that it's not 0
	assert.Zero(t, event1.Priority)
	assert.Zero(t, event1.Tags)
	assert.Zero(t, event1.AlertType)
	assert.Zero(t, event1.AggregationKey)
	assert.Zero(t, event1.SourceTypeName)

	event2 := agg.events[1]
	// No change is made
	assert.Equal(t, "Another event occurred", event2.Title)
	assert.Equal(t, "my-hostname", event2.Host)
	assert.Equal(t, int64(12345), event2.Ts)
	assert.Equal(t, metrics.EventPriorityNormal, event2.Priority)
	assert.Equal(t, []string{"foo", "bar"}, event2.Tags)
	assert.Equal(t, metrics.EventAlertTypeError, event2.AlertType)
	assert.Equal(t, "my_agg_key", event2.AggregationKey)
	assert.Equal(t, "custom_source_type", event2.SourceTypeName)
}

func TestSetHostname(t *testing.T) {
	resetAggregator()
	agg := InitAggregator(nil, "hostname")
	assert.Equal(t, "hostname", agg.hostname)
	agg.SetHostname("different-hostname")
	assert.Equal(t, "different-hostname", agg.hostname)
}
