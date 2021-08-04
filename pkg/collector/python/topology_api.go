// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at StackState (https://www.stackstate.com).
// Copyright 2021 StackState

// +build python

package python

import (
	"encoding/json"
	"fmt"
	"github.com/StackVista/stackstate-agent/pkg/batcher"
	"github.com/StackVista/stackstate-agent/pkg/collector/check"
	"github.com/StackVista/stackstate-agent/pkg/topology"
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
// rtloader/test/topology/topology.go

// SubmitComponent is the method exposed to Python scripts to submit topology component
//export SubmitComponent
func SubmitComponent(id *C.char, instanceKey *C.instance_key_t, _ *C.char, _ *C.char, data *C.char) {
	goCheckID := C.GoString(id)

	_instance := topology.Instance{
		Type: C.GoString(instanceKey.type_),
		URL:  C.GoString(instanceKey.url),
	}

	component := topology.Component{}
	rawComponent := C.GoString(data)
	err := json.Unmarshal([]byte(rawComponent), &component)

	if err == nil {
		batcher.GetBatcher().SubmitComponent(check.ID(goCheckID), _instance, component)
	} else {
		_ = log.Errorf("Empty topology component not sent. Raw: %v, Json: %v, Error: %v", rawComponent,
			component.JSONString(), err)
	}
}

// SubmitRelation is the method exposed to Python scripts to submit topology relation
//export SubmitRelation
func SubmitRelation(id *C.char, instanceKey *C.instance_key_t, _ *C.char, _ *C.char, _ *C.char, data *C.char) {
	goCheckID := C.GoString(id)

	_instance := topology.Instance{
		Type: C.GoString(instanceKey.type_),
		URL:  C.GoString(instanceKey.url),
	}

	relation := topology.Relation{}
	rawRelation := C.GoString(data)
	err := json.Unmarshal([]byte(rawRelation), &relation)

	if err == nil {
		relation.ExternalID = fmt.Sprintf("%s-%s-%s", relation.SourceID, relation.Type.Name, relation.TargetID)
		batcher.GetBatcher().SubmitRelation(check.ID(goCheckID), _instance, relation)
	} else {
		_ = log.Errorf("Empty topology relation not sent. Raw: %v, Json: %v, Error: %v", rawRelation,
			relation.JSONString(), err)
	}
}

// SubmitStartSnapshot starts a snapshot
//export SubmitStartSnapshot
func SubmitStartSnapshot(id *C.char, instanceKey *C.instance_key_t) {
	goCheckID := C.GoString(id)

	_instance := topology.Instance{
		Type: C.GoString(instanceKey.type_),
		URL:  C.GoString(instanceKey.url),
	}

	batcher.GetBatcher().SubmitStartSnapshot(check.ID(goCheckID), _instance)
}

// SubmitStopSnapshot stops a snapshot
//export SubmitStopSnapshot
func SubmitStopSnapshot(id *C.char, instanceKey *C.instance_key_t) {
	goCheckID := C.GoString(id)

	_instance := topology.Instance{
		Type: C.GoString(instanceKey.type_),
		URL:  C.GoString(instanceKey.url),
	}

	batcher.GetBatcher().SubmitStopSnapshot(check.ID(goCheckID), _instance)
}
