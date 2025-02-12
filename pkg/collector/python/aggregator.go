// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python

//nolint:revive // TODO(AML) Fix revive linter
package python

import (
	"context"
	"crypto/tls"
	"fmt"
	"sync"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/api/security"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	metricsevent "github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	grpcutil "github.com/DataDog/datadog-agent/pkg/util/grpc"
)

/*
#include <datadog_agent_rtloader.h>
#cgo !windows LDFLAGS: -L${SRCDIR}/../../../rtloader/build/rtloader -ldatadog-agent-rtloader -ldl
#cgo windows LDFLAGS: -L${SRCDIR}/../../../rtloader/build/rtloader -ldatadog-agent-rtloader -lstdc++ -static
#cgo CFLAGS: -I "${SRCDIR}/../../../rtloader/include"  -I "${SRCDIR}/../../../rtloader/common"
*/
import "C"

type metric struct {
	CheckId         string   `json:"check_id,omitempty"`
	Type            string   `json:"type,omitempty"`
	MetricName      string   `json:"metric_name,omitempty"`
	Value           float64  `json:"value,omitempty"`
	Hostname        string   `json:"hostname,omitempty"`
	Tags            []string `json:"tags,omitempty"`
	FlushFirstValue bool     `json:"flush_first_value,omitempty"`
}

var grpcOnce sync.Once
var grpcClient pb.AgentSecureClient

func getGRPCClient() pb.AgentSecureClient {
	grpcOnce.Do(func() {
		token, err := security.FetchAuthToken(pkgconfigsetup.Datadog())
		if err != nil {
			panic(err)
		}

		// NOTE: we're using InsecureSkipVerify because the gRPC server only
		// persists its TLS certs in memory, and we currently have no
		// infrastructure to make them available to clients. This is NOT
		// equivalent to grpc.WithInsecure(), since that assumes a non-TLS
		// connection.
		creds := credentials.NewTLS(&tls.Config{
			InsecureSkipVerify: true,
		})

		conn, err := grpc.NewClient(fmt.Sprintf(":%v", pkgconfigsetup.Datadog().GetInt("cmd_port")),
			grpc.WithTransportCredentials(creds),
			grpc.WithPerRPCCredentials(grpcutil.NewBearerTokenAuth(token)),
		)

		if err != nil {
			panic(err)
		}

		grpcClient = pb.NewAgentSecureClient(conn)
	})
	return grpcClient
}

// SubmitMetric is the method exposed to Python scripts to submit metrics
//
//export SubmitMetric
func SubmitMetric(checkID *C.char, metricType C.metric_type_t, metricName *C.char, value C.double, tags **C.char, hostname *C.char, flushFirstValue C.bool) {
	goCheckID := C.GoString(checkID)

	_name := C.GoString(metricName)
	_value := float64(value)
	_hostname := C.GoString(hostname)
	_tags := cStringArrayToSlice(tags)
	_flushFirstValue := bool(flushFirstValue)

	var _metricType string
	switch metricType {
	case C.DATADOG_AGENT_RTLOADER_GAUGE:
		_metricType = "GAUGE"
	case C.DATADOG_AGENT_RTLOADER_RATE:
		_metricType = "RATE"
	case C.DATADOG_AGENT_RTLOADER_COUNT:
		_metricType = "COUNT"
	case C.DATADOG_AGENT_RTLOADER_MONOTONIC_COUNT:
		_metricType = "MONOTONIC_COUNT"
	case C.DATADOG_AGENT_RTLOADER_COUNTER:
		_metricType = "COUNTER"
	case C.DATADOG_AGENT_RTLOADER_HISTOGRAM:
		_metricType = "HISTOGRAM"
	case C.DATADOG_AGENT_RTLOADER_HISTORATE:
		_metricType = "HISTORATE"
	}

	c := getGRPCClient()
	resp, err := c.SendCheckMetric(context.Background(), &pb.Metric{
		CheckId:         goCheckID,
		Type:            _metricType,
		MetricName:      _name,
		Value:           _value,
		Hostname:        _hostname,
		Tags:            _tags,
		FlushFirstValue: _flushFirstValue,
	})

	if err != nil {
		log.Errorf("Error submitting metric to the aggregator: %v", err)
		return
	}

	log.Debug(fmt.Sprintf("python to aggregator response: %s", resp.Status))
}

// SubmitServiceCheck is the method exposed to Python scripts to submit service checks
//
//export SubmitServiceCheck
func SubmitServiceCheck(checkID *C.char, scName *C.char, status C.int, tags **C.char, hostname *C.char, message *C.char) {
	goCheckID := C.GoString(checkID)

	checkContext, err := getCheckContext()
	if err != nil {
		log.Errorf("Python check context: %v", err)
		return
	}

	sender, err := checkContext.senderManager.GetSender(checkid.ID(goCheckID))
	if err != nil || sender == nil {
		log.Errorf("Error submitting metric to the Sender: %v", err)
		return
	}

	_name := C.GoString(scName)
	_status := servicecheck.ServiceCheckStatus(status)
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

	checkContext, err := getCheckContext()
	if err != nil {
		log.Errorf("Python check context: %v", err)
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
		Tags:           cStringArrayToSlice(event.tags),
		AlertType:      metricsevent.AlertType(eventParseString(event.alert_type, "alert_type")),
		AggregationKey: eventParseString(event.aggregation_key, "aggregation_key"),
		SourceTypeName: eventParseString(event.source_type_name, "source_type_name"),
		Ts:             int64(event.ts),
	}

	sender.Event(_event)
}

// SubmitHistogramBucket is the method exposed to Python scripts to submit metrics
//
//export SubmitHistogramBucket
func SubmitHistogramBucket(checkID *C.char, metricName *C.char, value C.longlong, lowerBound C.float, upperBound C.float, monotonic C.int, hostname *C.char, tags **C.char, flushFirstValue C.bool) {
	goCheckID := C.GoString(checkID)
	checkContext, err := getCheckContext()
	if err != nil {
		log.Errorf("Python check context: %v", err)
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
	_tags := cStringArrayToSlice(tags)
	_flushFirstValue := bool(flushFirstValue)

	sender.HistogramBucket(_name, _value, _lowerBound, _upperBound, _monotonic, _hostname, _tags, _flushFirstValue)
}

// SubmitEventPlatformEvent is the method exposed to Python scripts to submit event platform events
//
//export SubmitEventPlatformEvent
func SubmitEventPlatformEvent(checkID *C.char, rawEventPtr *C.char, rawEventSize C.int, eventType *C.char) {
	_checkID := C.GoString(checkID)
	checkContext, err := getCheckContext()
	if err != nil {
		log.Errorf("Python check context: %v", err)
		return
	}

	sender, err := checkContext.senderManager.GetSender(checkid.ID(_checkID))
	if err != nil || sender == nil {
		log.Errorf("Error submitting event platform event to the Sender: %v", err)
		return
	}
	sender.EventPlatformEvent(C.GoBytes(unsafe.Pointer(rawEventPtr), rawEventSize), C.GoString(eventType))
}
