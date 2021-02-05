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
	"github.com/StackVista/stackstate-agent/pkg/util/log"
	"gopkg.in/yaml.v2"
)

/*
#cgo !windows LDFLAGS: -ldatadog-agent-rtloader -ldl
#cgo windows LDFLAGS: -ldatadog-agent-rtloader -lstdc++ -static

#include "datadog_agent_rtloader.h"
#include "rtloader_mem.h"
*/
import "C"

// SubmitComponent is the method exposed to Python scripts to submit topology component
//export SubmitComponent
func SubmitComponent(id *C.char, instanceKey *C.instance_key_t, externalId *C.char, componentType *C.char, data *C.char) {
	goCheckID := C.GoString(id)

	_instance := topology.Instance{
		Type: C.GoString(instanceKey.type_),
		URL:  C.GoString(instanceKey.url),
	}

	_externalId := C.GoString(externalId)
	_componentType := C.GoString(componentType)
	_data := make(map[string]interface{})
	err := yaml.Unmarshal([]byte(C.GoString(data)), _data)
	if err != nil {
		log.Error(err)
		return
	}

	batcher.GetBatcher().SubmitComponent(check.ID(goCheckID),
		_instance,
		topology.Component{
			ExternalID: _externalId,
			Type:       topology.Type{Name: _componentType},
			Data:       _data,
		})
}

// SubmitRelation is the method exposed to Python scripts to submit topology relation
//export SubmitRelation
func SubmitRelation(id *C.char, instanceKey *C.instance_key_t, sourceId *C.char, targetId *C.char, relationType *C.char, data *C.char) {
	goCheckID := C.GoString(id)

	_instance := topology.Instance{
		Type: C.GoString(instanceKey.type_),
		URL:  C.GoString(instanceKey.url),
	}

	_sourceId := C.GoString(sourceId)
	_targetId := C.GoString(targetId)
	_relationType := C.GoString(relationType)

	_externalId := fmt.Sprintf("%s-%s-%s", _sourceId, _relationType, _targetId)

	_data := make(map[string]interface{})
	err := yaml.Unmarshal([]byte(C.GoString(data)), _data)
	if err != nil {
		log.Error(err)
		return
	}

	batcher.GetBatcher().SubmitRelation(check.ID(goCheckID),
		_instance,
		topology.Relation{
			ExternalID: _externalId,
			SourceID:   _sourceId,
			TargetID:   _targetId,
			Type:       topology.Type{Name: _relationType},
			Data:       _data,
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
