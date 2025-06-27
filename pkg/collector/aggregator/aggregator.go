// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

/*
#include <datadog_agent_rtloader.h>
#cgo !windows LDFLAGS: -L${SRCDIR}/../../../rtloader/build/rtloader -ldatadog-agent-rtloader -ldl
#cgo windows LDFLAGS: -L${SRCDIR}/../../../rtloader/build/rtloader -ldatadog-agent-rtloader -lstdc++ -static
#cgo CFLAGS: -I "${SRCDIR}/../../../rtloader/include"  -I "${SRCDIR}/../../../rtloader/common"
*/
import "C"

// SubmitMetricRtLoader acts as an interface for RTLoader to submit metrics.
//
//export SubmitMetricRtLoader
func SubmitMetricRtLoader(checkID *C.char, metricType C.metric_type_t, metricName *C.char, value C.double, tags **C.char, hostname *C.char, flushFirstValue C.bool) {
	goCheckID := C.GoString(checkID)

	checkContext, err := getCheckContext()
	if err != nil {
		log.Errorf("Check context: %v", err)
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

	sender.Commit()
}
