// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at StackState (https://www.stackstate.com).
// Copyright 2021 StackState

// +build python

package python

import (
	"fmt"
	"github.com/StackVista/stackstate-agent/pkg/batcher"
	"github.com/StackVista/stackstate-agent/pkg/collector/check"
	"github.com/StackVista/stackstate-agent/pkg/topology"
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
func SubmitComponent(id *C.char, instanceKey *C.instance_key_t, externalID *C.char, componentType *C.char, data *C.char) {
	goCheckID := C.GoString(id)

	_instance := topology.Instance{
		Type: C.GoString(instanceKey.type_),
		URL:  C.GoString(instanceKey.url),
	}

	_externalID := C.GoString(externalID)
	_componentType := C.GoString(componentType)
	_json := yamlDataToJSON(data)

	batcher.GetBatcher().SubmitComponent(check.ID(goCheckID),
		_instance,
		topology.Component{
			ExternalID: _externalID,
			Type:       topology.Type{Name: _componentType},
			Data:       _json,
		})
}

// SubmitRelation is the method exposed to Python scripts to submit topology relation
//export SubmitRelation
func SubmitRelation(id *C.char, instanceKey *C.instance_key_t, sourceID *C.char, targetID *C.char, relationType *C.char, data *C.char) {
	goCheckID := C.GoString(id)

	_instance := topology.Instance{
		Type: C.GoString(instanceKey.type_),
		URL:  C.GoString(instanceKey.url),
	}

	_sourceID := C.GoString(sourceID)
	_targetID := C.GoString(targetID)
	_relationType := C.GoString(relationType)
	_externalID := fmt.Sprintf("%s-%s-%s", _sourceID, _relationType, _targetID)
	_json := yamlDataToJSON(data)

	batcher.GetBatcher().SubmitRelation(check.ID(goCheckID),
		_instance,
		topology.Relation{
			ExternalID: _externalID,
			SourceID:   _sourceID,
			TargetID:   _targetID,
			Type:       topology.Type{Name: _relationType},
			Data:       _json,
		})
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
