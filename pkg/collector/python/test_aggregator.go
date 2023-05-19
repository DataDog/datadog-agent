// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python && test

package python

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// #include <datadog_agent_rtloader.h>
import "C"

func testSubmitMetric(t *testing.T) {
	sender := mocksender.NewMockSender(check.ID("testID"))
	sender.SetupAcceptAll()

	cTags := []*C.char{C.CString("tag1"), C.CString("tag2"), nil}
	SubmitMetric(C.CString("testID"),
		C.DATADOG_AGENT_RTLOADER_GAUGE,
		C.CString("test_gauge"),
		C.double(21),
		&cTags[0],
		C.CString("my_hostname"),
		C.bool(false))
	SubmitMetric(C.CString("testID"),
		C.DATADOG_AGENT_RTLOADER_RATE,
		C.CString("test_rate"),
		C.double(21),
		&cTags[0],
		C.CString("my_hostname"),
		C.bool(false))
	SubmitMetric(C.CString("testID"),
		C.DATADOG_AGENT_RTLOADER_COUNT,
		C.CString("test_count"),
		C.double(21),
		&cTags[0],
		C.CString("my_hostname"),
		C.bool(false))
	SubmitMetric(C.CString("testID"),
		C.DATADOG_AGENT_RTLOADER_MONOTONIC_COUNT,
		C.CString("test_monotonic_count"),
		C.double(21),
		&cTags[0],
		C.CString("my_hostname"),
		C.bool(false))
	SubmitMetric(C.CString("testID"),
		C.DATADOG_AGENT_RTLOADER_MONOTONIC_COUNT,
		C.CString("test_monotonic_count_flush_first_value"),
		C.double(21),
		&cTags[0],
		C.CString("my_hostname"),
		C.bool(true))
	SubmitMetric(C.CString("testID"),
		C.DATADOG_AGENT_RTLOADER_COUNTER,
		C.CString("test_counter"),
		C.double(21),
		&cTags[0],
		C.CString("my_hostname"),
		C.bool(false))
	SubmitMetric(C.CString("testID"),
		C.DATADOG_AGENT_RTLOADER_HISTOGRAM,
		C.CString("test_histogram"),
		C.double(21),
		&cTags[0],
		C.CString("my_hostname"),
		C.bool(false))
	SubmitMetric(C.CString("testID"),
		C.DATADOG_AGENT_RTLOADER_HISTORATE,
		C.CString("test_historate"),
		C.double(21),
		&cTags[0],
		C.CString("my_hostname"),
		C.bool(false))

	sender.AssertMetric(t, "Gauge", "test_gauge", 21, "my_hostname", []string{"tag1", "tag2"})
	sender.AssertMetric(t, "Rate", "test_rate", 21, "my_hostname", []string{"tag1", "tag2"})
	sender.AssertMetric(t, "Count", "test_count", 21, "my_hostname", []string{"tag1", "tag2"})
	sender.AssertMonotonicCount(t, "MonotonicCountWithFlushFirstValue", "test_monotonic_count", 21, "my_hostname", []string{"tag1", "tag2"}, false)
	sender.AssertMonotonicCount(t, "MonotonicCountWithFlushFirstValue", "test_monotonic_count_flush_first_value", 21, "my_hostname", []string{"tag1", "tag2"}, true)
	sender.AssertMetric(t, "Counter", "test_counter", 21, "my_hostname", []string{"tag1", "tag2"})
	sender.AssertMetric(t, "Histogram", "test_histogram", 21, "my_hostname", []string{"tag1", "tag2"})
	sender.AssertMetric(t, "Historate", "test_historate", 21, "my_hostname", []string{"tag1", "tag2"})
}

func testSubmitMetricEmptyTags(t *testing.T) {
	sender := mocksender.NewMockSender(check.ID("testID"))
	sender.SetupAcceptAll()

	cTags := []*C.char{nil}
	SubmitMetric(C.CString("testID"),
		C.DATADOG_AGENT_RTLOADER_GAUGE,
		C.CString("test_gauge"),
		C.double(21),
		&cTags[0],
		C.CString("my_hostname"),
		C.bool(false))

	sender.AssertMetric(t, "Gauge", "test_gauge", 21, "my_hostname", nil)
}

func testSubmitMetricEmptyHostname(t *testing.T) {
	sender := mocksender.NewMockSender(check.ID("testID"))
	sender.SetupAcceptAll()

	cTags := []*C.char{nil}
	SubmitMetric(C.CString("testID"),
		C.DATADOG_AGENT_RTLOADER_GAUGE,
		C.CString("test_gauge"),
		C.double(21),
		&cTags[0],
		nil,
		C.bool(false))

	sender.AssertMetric(t, "Gauge", "test_gauge", 21, "", nil)
}

func testSubmitServiceCheck(t *testing.T) {
	sender := mocksender.NewMockSender(check.ID("testID"))
	sender.SetupAcceptAll()

	cTags := []*C.char{C.CString("tag1"), C.CString("tag2"), nil}
	SubmitServiceCheck(C.CString("testID"),
		C.CString("service_name"),
		C.int(1),
		&cTags[0],
		C.CString("my_hostname"),
		C.CString("my_message"))

	sender.AssertServiceCheck(t, "service_name", metrics.ServiceCheckWarning, "my_hostname", []string{"tag1", "tag2"}, "my_message")
}

func testSubmitServiceCheckEmptyTag(t *testing.T) {
	sender := mocksender.NewMockSender(check.ID("testID"))
	sender.SetupAcceptAll()

	cTags := []*C.char{nil}
	SubmitServiceCheck(C.CString("testID"),
		C.CString("service_name"),
		C.int(1),
		&cTags[0],
		C.CString("my_hostname"),
		C.CString("my_message"))

	sender.AssertServiceCheck(t, "service_name", metrics.ServiceCheckWarning, "my_hostname", nil, "my_message")
}

func testSubmitServiceCheckEmptyHostame(t *testing.T) {
	sender := mocksender.NewMockSender(check.ID("testID"))
	sender.SetupAcceptAll()

	cTags := []*C.char{nil}
	SubmitServiceCheck(C.CString("testID"),
		C.CString("service_name"),
		C.int(1),
		&cTags[0],
		nil,
		C.CString("my_message"))

	sender.AssertServiceCheck(t, "service_name", metrics.ServiceCheckWarning, "", nil, "my_message")
}

func testSubmitEvent(t *testing.T) {
	sender := mocksender.NewMockSender(check.ID("testID"))
	sender.SetupAcceptAll()

	ev := C.event_t{}
	ev.title = C.CString("ev_title")
	ev.text = C.CString("ev_text")
	ev.ts = 21
	ev.priority = C.CString("ev_priority")
	ev.host = C.CString("ev_host")
	ev.alert_type = C.CString("alert_type")
	ev.aggregation_key = C.CString("aggregation_key")
	ev.source_type_name = C.CString("source_type")
	ev.event_type = C.CString("event_type")
	tags := []*C.char{C.CString("tag1"), C.CString("tag2"), nil}
	ev.tags = &tags[0]

	SubmitEvent(C.CString("testID"), &ev)

	expectedEvent := metrics.Event{
		Title:          "ev_title",
		Text:           "ev_text",
		Ts:             21,
		Priority:       "ev_priority",
		Host:           "ev_host",
		Tags:           []string{"tag1", "tag2"},
		AlertType:      "alert_type",
		AggregationKey: "aggregation_key",
		SourceTypeName: "source_type",
	}
	sender.AssertEvent(t, expectedEvent, 0)
}

func testSubmitHistogramBucket(t *testing.T) {
	sender := mocksender.NewMockSender(check.ID("testID"))
	sender.SetupAcceptAll()

	cTags := []*C.char{C.CString("tag1"), C.CString("tag2"), nil}
	SubmitHistogramBucket(
		C.CString("testID"),
		C.CString("test_histogram"),
		C.longlong(42),
		C.float(1.0),
		C.float(2.0),
		C.int(1),
		C.CString("my_hostname"),
		&cTags[0],
		true,
	)

	sender.AssertHistogramBucket(t, "HistogramBucket", "test_histogram", 42, 1.0, 2.0, true, "my_hostname", []string{"tag1", "tag2"}, true)
}

func testSubmitEventPlatformEvent(t *testing.T) {
	sender := mocksender.NewMockSender("testID")
	sender.SetupAcceptAll()
	SubmitEventPlatformEvent(
		C.CString("testID"),
		C.CString("raw-event"),
		C.int(len("raw-event")),
		C.CString("dbm-sample"),
	)

	sender.AssertEventPlatformEvent(t, []byte("raw-event"), "dbm-sample")
}
