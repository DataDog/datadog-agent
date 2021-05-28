// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at StackState (https://www.stackstate.com).
// Copyright 2021 StackState

// +build python

package python

import (
	"github.com/StackVista/stackstate-agent/pkg/batcher"
	"github.com/StackVista/stackstate-agent/pkg/collector/check"
	"github.com/StackVista/stackstate-agent/pkg/health"
	"github.com/StackVista/stackstate-agent/pkg/util/log"
)

/*
#cgo !windows LDFLAGS: -ldatadog-agent-rtloader -ldl
#cgo windows LDFLAGS: -ldatadog-agent-rtloader -lstdc++ -static

#include "datadog_agent_rtloader.h"
#include "rtloader_mem.h"
*/
import "C"

// NOTE
// Beware that any changes made here MUST be reflected also in the test implementation
// rtloader/test/health/health.go

// SubmitHealthCheckData is the method exposed to Python scripts to submit health check data
//export SubmitHealthCheckData
func SubmitHealthCheckData(id *C.char, healthStream *C.health_stream_t, data *C.char) {
	goCheckID := C.GoString(id)
	_stream := convertStream(healthStream)
	_json, err := unsafeParseYamlToMap(data)

	if err == nil {
		if len(_json) != 0 {
			batcher.GetBatcher().SubmitHealthCheckData(check.ID(goCheckID), _stream, _json)
		} else {
			_ = log.Errorf("Empty json submitted to as check data, this is not allowed, data will not be forwarded.")
		}
	} else {
		_ = log.Errorf("Error converting health data yaml to go: Error: %v", err)
	}
}

// SubmitHealthStartSnapshot starts a health snapshot
//export SubmitHealthStartSnapshot
func SubmitHealthStartSnapshot(id *C.char, healthStream *C.health_stream_t, expirySeconds C.int, repeatIntervalSeconds C.int) {
	goCheckID := C.GoString(id)
	_stream := convertStream(healthStream)

	batcher.GetBatcher().SubmitHealthStartSnapshot(check.ID(goCheckID), _stream, int(repeatIntervalSeconds), int(expirySeconds))
}

// SubmitHealthStopSnapshot stops a health snapshot
//export SubmitHealthStopSnapshot
func SubmitHealthStopSnapshot(id *C.char, healthStream *C.health_stream_t) {
	goCheckID := C.GoString(id)
	_stream := convertStream(healthStream)

	batcher.GetBatcher().SubmitHealthStopSnapshot(check.ID(goCheckID), _stream)
}

func convertStream(healthStream *C.health_stream_t) health.Stream {
	_subStream := C.GoString(healthStream.sub_stream)
	if _subStream == "" {
		return health.Stream{
			Urn: C.GoString(healthStream.urn),
		}
	}
	return health.Stream{
		Urn:       C.GoString(healthStream.urn),
		SubStream: _subStream,
	}
}
