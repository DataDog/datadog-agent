// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package aggregator provides an API for checks run through Cgo
package aggregator

import (
	"unsafe"

	metricsevent "github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

/*
#cgo CFLAGS: -I "${SRCDIR}/../../../rtloader/include"

#include <stdbool.h>

#include "rtloader_types.h"
*/
import "C"

// SubmitMetric is the method exposed to scripts to submit metrics
//
//export SubmitMetric
func SubmitMetric(checkID *C.char, metricType C.metric_type_t, metricName *C.char, value C.double, tags **C.char, hostname *C.char, flushFirstValue C.bool) {
	SubmitMetricForCheck(
		C.GoString(checkID),
		int(metricType),
		C.GoString(metricName),
		float64(value),
		CStringArrayToSlice(unsafe.Pointer(tags)),
		C.GoString(hostname),
		bool(flushFirstValue),
	)
}

// SubmitServiceCheck is the method exposed to scripts to submit service checks
//
//export SubmitServiceCheck
func SubmitServiceCheck(checkID *C.char, scName *C.char, status C.int, tags **C.char, hostname *C.char, message *C.char) {
	SubmitServiceCheckForCheck(
		C.GoString(checkID),
		C.GoString(scName),
		servicecheck.ServiceCheckStatus(status),
		CStringArrayToSlice(unsafe.Pointer(tags)),
		C.GoString(hostname),
		C.GoString(message),
	)
}

func eventParseString(value *C.char, fieldName string) string {
	if value == nil {
		log.Tracef("Can't parse value for key '%s' in event submitted from python check", fieldName)
		return ""
	}
	return C.GoString(value)
}

// SubmitEvent is the method exposed to scripts to submit events
//
//export SubmitEvent
func SubmitEvent(checkID *C.char, event *C.event_t) {
	_event := metricsevent.Event{
		Title:          eventParseString(event.title, "msg_title"),
		Text:           eventParseString(event.text, "msg_text"),
		Priority:       metricsevent.Priority(eventParseString(event.priority, "priority")),
		Host:           eventParseString(event.host, "host"),
		Tags:           CStringArrayToSlice(unsafe.Pointer(event.tags)),
		AlertType:      metricsevent.AlertType(eventParseString(event.alert_type, "alert_type")),
		AggregationKey: eventParseString(event.aggregation_key, "aggregation_key"),
		SourceTypeName: eventParseString(event.source_type_name, "source_type_name"),
		Ts:             int64(event.ts),
	}

	SubmitEventForCheck(C.GoString(checkID), _event)
}

// SubmitHistogramBucket is the method exposed to scripts to submit metrics
//
//export SubmitHistogramBucket
func SubmitHistogramBucket(checkID *C.char, metricName *C.char, value C.longlong, lowerBound C.float, upperBound C.float, monotonic C.int, hostname *C.char, tags **C.char, flushFirstValue C.bool) {
	SubmitHistogramBucketForCheck(
		C.GoString(checkID),
		C.GoString(metricName),
		int64(value),
		float64(lowerBound),
		float64(upperBound),
		monotonic != 0,
		C.GoString(hostname),
		CStringArrayToSlice(unsafe.Pointer(tags)),
		bool(flushFirstValue),
	)
}

// SubmitEventPlatformEvent is the method exposed to scripts to submit event platform events
//
//export SubmitEventPlatformEvent
func SubmitEventPlatformEvent(checkID *C.char, rawEventPtr *C.char, rawEventSize C.int, eventType *C.char) {
	SubmitEventPlatformEventForCheck(
		C.GoString(checkID),
		C.GoBytes(unsafe.Pointer(rawEventPtr), rawEventSize),
		C.GoString(eventType),
	)
}
