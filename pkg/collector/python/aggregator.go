// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python

package python

import (
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	chk "github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

/*
#include <datadog_agent_rtloader.h>
#cgo !windows LDFLAGS: -L${SRCDIR}/../../../rtloader/build/rtloader -ldatadog-agent-rtloader -ldl
#cgo windows LDFLAGS: -L${SRCDIR}/../../../rtloader/build/rtloader -ldatadog-agent-rtloader -lstdc++ -static
#cgo CFLAGS: -I "${SRCDIR}/../../../rtloader/include"  -I "${SRCDIR}/../../../rtloader/common"
*/
import "C"

// SubmitMetric is the method exposed to Python scripts to submit metrics
//
//export SubmitMetric
func SubmitMetric(checkID *C.char, metricType C.metric_type_t, metricName *C.char, value C.double, tags **C.char, hostname *C.char, flushFirstValue C.bool) {
	goCheckID := C.GoString(checkID)

	sender, err := aggregator.GetSender(chk.ID(goCheckID))
	if err != nil || sender == nil {
		log.Errorf("Error submitting metric to the Sender: %v", err)
		return
	}

	_name := C.GoString(metricName)
	_value := float64(value)
	_hostname := C.GoString(hostname)
	_tags := cStringArrayToSlice(tags)
	_flushFirstValue := bool(flushFirstValue)

	switch metricType {
	case C.DATADOG_AGENT_RTLOADER_GAUGE:
		sender.Gauge(_name, _value, _hostname, _tags)
	case C.DATADOG_AGENT_RTLOADER_RATE:
		sender.Rate(_name, _value, _hostname, _tags)
	case C.DATADOG_AGENT_RTLOADER_COUNT:
		sender.Count(_name, _value, _hostname, _tags)
	case C.DATADOG_AGENT_RTLOADER_MONOTONIC_COUNT:
		sender.MonotonicCountWithFlushFirstValue(_name, _value, _hostname, _tags, _flushFirstValue)
	case C.DATADOG_AGENT_RTLOADER_COUNTER:
		sender.Counter(_name, _value, _hostname, _tags)
	case C.DATADOG_AGENT_RTLOADER_HISTOGRAM:
		sender.Histogram(_name, _value, _hostname, _tags)
	case C.DATADOG_AGENT_RTLOADER_HISTORATE:
		sender.Historate(_name, _value, _hostname, _tags)
	}
}

// SubmitServiceCheck is the method exposed to Python scripts to submit service checks
//
//export SubmitServiceCheck
func SubmitServiceCheck(checkID *C.char, scName *C.char, status C.int, tags **C.char, hostname *C.char, message *C.char) {
	goCheckID := C.GoString(checkID)

	sender, err := aggregator.GetSender(chk.ID(goCheckID))
	if err != nil || sender == nil {
		log.Errorf("Error submitting metric to the Sender: %v", err)
		return
	}

	_name := C.GoString(scName)
	_status := metrics.ServiceCheckStatus(status)
	_tags := cStringArrayToSlice(tags)
	_hostname := C.GoString(hostname)
	_message := C.GoString(message)

	sender.ServiceCheck(_name, _status, _hostname, _tags, _message)
}

func eventParseString(value *C.char, fieldName string) string {
	if value == nil {
		log.Tracef("Can't parse value for key '%s' in event submitted from python check", fieldName)
		return ""
	}
	return C.GoString(value)
}

// SubmitEvent is the method exposed to Python scripts to submit events
//
//export SubmitEvent
func SubmitEvent(checkID *C.char, event *C.event_t) {
	goCheckID := C.GoString(checkID)

	sender, err := aggregator.GetSender(chk.ID(goCheckID))
	if err != nil || sender == nil {
		log.Errorf("Error submitting metric to the Sender: %v", err)
		return
	}

	_event := metrics.Event{
		Title:          eventParseString(event.title, "msg_title"),
		Text:           eventParseString(event.text, "msg_text"),
		Priority:       metrics.EventPriority(eventParseString(event.priority, "priority")),
		Host:           eventParseString(event.host, "host"),
		Tags:           cStringArrayToSlice(event.tags),
		AlertType:      metrics.EventAlertType(eventParseString(event.alert_type, "alert_type")),
		AggregationKey: eventParseString(event.aggregation_key, "aggregation_key"),
		SourceTypeName: eventParseString(event.source_type_name, "source_type_name"),
		Ts:             int64(event.ts),
	}

	sender.Event(_event)
	return
}

// SubmitHistogramBucket is the method exposed to Python scripts to submit metrics
//
//export SubmitHistogramBucket
func SubmitHistogramBucket(checkID *C.char, metricName *C.char, value C.longlong, lowerBound C.float, upperBound C.float, monotonic C.int, hostname *C.char, tags **C.char, flushFirstValue C.bool) {
	goCheckID := C.GoString(checkID)
	sender, err := aggregator.GetSender(chk.ID(goCheckID))
	if err != nil || sender == nil {
		log.Errorf("Error submitting histogram bucket to the Sender: %v", err)
		return
	}

	_name := C.GoString(metricName)
	_value := int64(value)
	_lowerBound := float64(lowerBound)
	_upperBound := float64(upperBound)
	_monotonic := (monotonic != 0)
	_hostname := C.GoString(hostname)
	_tags := cStringArrayToSlice(tags)
	_flushFirstValue := bool(flushFirstValue)

	sender.HistogramBucket(_name, _value, _lowerBound, _upperBound, _monotonic, _hostname, _tags, _flushFirstValue)
}

// SubmitEventPlatformEvent is the method exposed to Python scripts to submit event platform events
//
//export SubmitEventPlatformEvent
func SubmitEventPlatformEvent(checkID *C.char, rawEventPtr *C.char, rawEventSize C.int, eventType *C.char) {
	_checkID := C.GoString(checkID)
	sender, err := aggregator.GetSender(chk.ID(_checkID))
	if err != nil || sender == nil {
		log.Errorf("Error submitting event platform event to the Sender: %v", err)
		return
	}
	sender.EventPlatformEvent(C.GoBytes(unsafe.Pointer(rawEventPtr), rawEventSize), C.GoString(eventType))
}
