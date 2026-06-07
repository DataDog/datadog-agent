// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package aggregator provides an API for checks run through Cgo
package aggregator

import (
	"encoding/json"
	"time"
	"unsafe"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	metricsevent "github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/sds"
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
	goCheckID := C.GoString(checkID)

	checkContext, err := GetCheckContext()
	if err != nil {
		log.Errorf("check context: %v", err)
		return
	}

	sender, err := checkContext.senderManager.GetSender(checkid.ID(goCheckID))
	if err != nil || sender == nil {
		log.Errorf("Error submitting metric to the Sender: %v", err)
		return
	}

	_name := C.GoString(metricName)
	_value := float64(value)
	_hostname := C.GoString(hostname)
	_tags := CStringArrayToSlice(unsafe.Pointer(tags))
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

// SubmitServiceCheck is the method exposed to scripts to submit service checks
//
//export SubmitServiceCheck
func SubmitServiceCheck(checkID *C.char, scName *C.char, status C.int, tags **C.char, hostname *C.char, message *C.char) {
	goCheckID := C.GoString(checkID)

	checkContext, err := GetCheckContext()
	if err != nil {
		log.Errorf("check context: %v", err)
		return
	}

	sender, err := checkContext.senderManager.GetSender(checkid.ID(goCheckID))
	if err != nil || sender == nil {
		log.Errorf("Error submitting metric to the Sender: %v", err)
		return
	}

	_name := C.GoString(scName)
	_status := servicecheck.ServiceCheckStatus(status)
	_tags := CStringArrayToSlice(unsafe.Pointer(tags))
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

// SubmitEvent is the method exposed to scripts to submit events
//
//export SubmitEvent
func SubmitEvent(checkID *C.char, event *C.event_t) {
	goCheckID := C.GoString(checkID)

	checkContext, err := GetCheckContext()
	if err != nil {
		log.Errorf("check context: %v", err)
		return
	}

	sender, err := checkContext.senderManager.GetSender(checkid.ID(goCheckID))
	if err != nil || sender == nil {
		log.Errorf("Error submitting metric to the Sender: %v", err)
		return
	}

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

	sender.Event(_event)
}

// SubmitHistogramBucket is the method exposed to scripts to submit metrics
//
//export SubmitHistogramBucket
func SubmitHistogramBucket(checkID *C.char, metricName *C.char, value C.longlong, lowerBound C.float, upperBound C.float, monotonic C.int, hostname *C.char, tags **C.char, flushFirstValue C.bool) {
	goCheckID := C.GoString(checkID)
	checkContext, err := GetCheckContext()
	if err != nil {
		log.Errorf("check context: %v", err)
		return
	}

	sender, err := checkContext.senderManager.GetSender(checkid.ID(goCheckID))
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
	_tags := CStringArrayToSlice(unsafe.Pointer(tags))
	_flushFirstValue := bool(flushFirstValue)

	sender.OpenmetricsBucket(_name, _value, _lowerBound, _upperBound, _monotonic, _hostname, _tags, _flushFirstValue)
}

// SubmitEventPlatformEvent is the method exposed to scripts to submit event platform events
//
//export SubmitEventPlatformEvent
func SubmitEventPlatformEvent(checkID *C.char, rawEventPtr *C.char, rawEventSize C.int, eventType *C.char) {
	_checkID := C.GoString(checkID)
	checkContext, err := GetCheckContext()
	if err != nil {
		log.Errorf("check context: %v", err)
		return
	}

	sender, err := checkContext.senderManager.GetSender(checkid.ID(_checkID))
	if err != nil || sender == nil {
		log.Errorf("Error submitting event platform event to the Sender: %v", err)
		return
	}
	sender.EventPlatformEvent(C.GoBytes(unsafe.Pointer(rawEventPtr), rawEventSize), C.GoString(eventType))
}

// ScanAndSubmitEventPlatformEvent scans the raw event with the Sensitive Data
// Scanner and submits the processed (e.g. redacted) event to the event platform.
// This lets integrations forward raw extracted data to the Agent and have the
// Agent perform the sensitive-data analysis before the event is shipped to the
// Datadog backend.
//
//export ScanAndSubmitEventPlatformEvent
func ScanAndSubmitEventPlatformEvent(checkID *C.char, rawEventPtr *C.char, rawEventSize C.int, eventType *C.char) {
	_checkID := C.GoString(checkID)
	checkContext, err := GetCheckContext()
	if err != nil {
		log.Errorf("check context: %v", err)
		return
	}

	sender, err := checkContext.senderManager.GetSender(checkid.ID(_checkID))
	if err != nil || sender == nil {
		log.Errorf("Error submitting event platform event to the Sender: %v", err)
		return
	}

	rawEvent := C.GoBytes(unsafe.Pointer(rawEventPtr), rawEventSize)

	var event dbScanEvent
	if err := json.Unmarshal(rawEvent, &event); err != nil {
		log.Errorf("Error parsing event platform event for SDS scanning: %v", err)
		return
	}

	// Scan only the rows: each row is scanned as a structured map event so we
	// can attribute matches back to their row index and column.
	start := time.Now()
	rowMatches := make([][]sds.Match, len(event.Rows))
	for i, row := range event.Rows {
		matches, scanErr := sds.ScanMap(row)
		if scanErr != nil {
			log.Errorf("Error scanning row %d with SDS: %v", i, scanErr)
			continue
		}
		rowMatches[i] = matches
	}
	scanDuration := time.Since(start)

	p := buildSdsResultPayload(event, rowMatches, scanDuration)

	// Forward the protobuf-encoded payload to the sds-result intake.
	if raw, err := proto.Marshal(p); err != nil {
		log.Errorf("Error encoding SDS result protobuf payload: %v", err)
	} else {
		sender.EventPlatformEvent(raw, eventplatform.EventTypeSDSResult)
	}

	// Also submit a Datadog event with the JSON-rendered payload, wrapped in
	// Datadog markdown delimiters with a fenced JSON code block so the Event
	// Explorer renders it as formatted JSON.
	payload, err := protojson.MarshalOptions{Multiline: true, Indent: "  ", UseProtoNames: true}.Marshal(p)
	if err != nil {
		log.Errorf("Error encoding SDS result payload: %v", err)
		return
	}
	text := "%%%\n```json\n" + string(payload) + "\n```\n%%%"
	sender.Event(metricsevent.Event{
		Title:     "Data security analysis",
		Text:      text,
		Priority:  metricsevent.PriorityNormal,
		AlertType: metricsevent.AlertTypeInfo,
		EventType: C.GoString(eventType),
		Host:      event.Host,
		Tags:      []string{"table:" + event.Table, "database_instance:" + event.DatabaseInstance},
		Ts:        time.Now().Unix(),
	})
}
