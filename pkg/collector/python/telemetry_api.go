// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at StackState (https://www.stackstate.com).
// Copyright 2021 StackState

// +build python

package python

import (
	"github.com/StackVista/stackstate-agent/pkg/aggregator"
	"github.com/StackVista/stackstate-agent/pkg/collector/check"
	"github.com/StackVista/stackstate-agent/pkg/metrics"
	"github.com/StackVista/stackstate-agent/pkg/util/log"
	"github.com/mitchellh/mapstructure"
	"gopkg.in/yaml.v2"
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
// rtloader/test/telemetry/telemetry.go

// SubmitTopologyEvent is the method exposed to Python scripts to submit topology event
//export SubmitTopologyEvent
func SubmitTopologyEvent(id *C.char, data *C.char) {
	goCheckID := C.GoString(id)

	var sender aggregator.Sender
	var err error

	sender, err = aggregator.GetSender(check.ID(goCheckID))
	if err != nil || sender == nil {
		log.Errorf("Error submitting topology event to the Sender: %v", err)
		return
	}

	_data := make(map[string]interface{})
	err = yaml.Unmarshal([]byte(C.GoString(data)), _data)
	if err != nil {
		log.Error(err)
		return
	}

	var topologyEvent metrics.Event
	err = mapstructure.Decode(_data, &topologyEvent)
	if err != nil {
		log.Error(err)
		return
	}

	sender.Event(topologyEvent)
	return
}
