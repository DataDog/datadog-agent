// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build cpython

package py

import (
	"errors"
	"fmt"
	"github.com/StackVista/stackstate-agent/pkg/batcher"
	"github.com/StackVista/stackstate-agent/pkg/collector/check"
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"unsafe"

	"github.com/sbinet/go-python"

	"github.com/StackVista/stackstate-agent/pkg/util/log"
)

// #cgo pkg-config: python-2.7
// #cgo windows LDFLAGS: -Wl,--allow-multiple-definition
// #include "stackstate_api.h"
// #include "api.h"
// #include "stdlib.h"
// #include <Python.h>
import "C"

// SubmitComponent is the method exposed to Python scripts to submit components
//export SubmitComponent
func SubmitComponent(chk *C.PyObject, checkID *C.char, instanceKey *C.PyObject, externalId *C.char, componentType *C.char, data *C.PyObject) *C.PyObject {

	goCheckID := C.GoString(checkID)

	_instance, err := extractInstance(instanceKey, goCheckID)
	if err != nil {
		log.Error(err)
		return nil
	}

	_externalId := C.GoString(externalId)
	_componentType := C.GoString(componentType)
	_data, err := extractStructureFromObject(data, goCheckID)
	if err != nil {
		log.Error(err)
		return nil
	}

	batcher.GetBatcher().SubmitComponent(check.ID(goCheckID),
		*_instance,
		topology.Component{
			ExternalId: _externalId,
			Type:       topology.Type{Name: _componentType},
			Data:       _data,
		})

	return C._none()
}

// SubmitRelation is the method exposed to Python scripts to submit relations
//export SubmitRelation
func SubmitRelation(chk *C.PyObject, checkID *C.char, instanceKey *C.PyObject, sourceId *C.char, targetId *C.char, relationType *C.char, data *C.PyObject) *C.PyObject {

	goCheckID := C.GoString(checkID)

	_instance, err := extractInstance(instanceKey, goCheckID)
	if err != nil {
		log.Error(err)
		return nil
	}

	_sourceId := C.GoString(sourceId)
	_targetId := C.GoString(targetId)
	_relationType := C.GoString(relationType)

	_externalId := fmt.Sprintf("%s-%s-%s", _sourceId, _relationType, _targetId)

	_data, err := extractStructureFromObject(data, goCheckID)
	if err != nil {
		log.Error(err)
		return nil
	}

	batcher.GetBatcher().SubmitRelation(check.ID(goCheckID),
		*_instance,
		topology.Relation{
			ExternalId: _externalId,
			SourceId:   _sourceId,
			TargetId:   _targetId,
			Type:       topology.Type{Name: _relationType},
			Data:       _data,
		})

	return C._none()
}

// SubmitStartSnapshot starts a snapshot
//export SubmitStartSnapshot
func SubmitStartSnapshot(chk *C.PyObject, checkID *C.char, instanceKey *C.PyObject) *C.PyObject {

	goCheckID := C.GoString(checkID)

	_instance, err := extractInstance(instanceKey, goCheckID)
	if err != nil {
		log.Error(err)
		return nil
	}

	batcher.GetBatcher().SubmitStartSnapshot(check.ID(goCheckID), *_instance)

	return C._none()
}

// SubmitStopSnapshot starts a snapshot
//export SubmitStopSnapshot
func SubmitStopSnapshot(chk *C.PyObject, checkID *C.char, instanceKey *C.PyObject) *C.PyObject {

	goCheckID := C.GoString(checkID)

	_instance, err := extractInstance(instanceKey, goCheckID)
	if err != nil {
		log.Error(err)
		return nil
	}

	batcher.GetBatcher().SubmitStopSnapshot(check.ID(goCheckID), *_instance)

	return C._none()
}

func extractInstance(instanceKey *C.PyObject, checkId string) (*topology.Instance, error) {
	_instanceKey, err := extractStructureFromObject(instanceKey, checkId)
	if err != nil {
		return nil, err
	}

	if _, ok := _instanceKey["type"]; !ok {
		return nil, fmt.Errorf("'type' field not found in instance specification")
	}

	if _, ok := _instanceKey["url"]; !ok {
		return nil, fmt.Errorf("'url' field not found in instance specification")
	}

	return &topology.Instance{
		Type: _instanceKey["type"].(string),
		Url:  _instanceKey["url"].(string),
	}, nil
}

// extractStructureFromComponent parses a python object into an unstructured go map
func extractStructureFromObject(data *C.PyObject, checkID string) (_data map[string]interface{}, err error) {
	glock := newStickyLock()
	defer glock.unlock()

	if isNone(data) {
		return make(map[string]interface{}), nil
	}

	if int(C._PyDict_Check(data)) == 0 {
		typeName := C.GoString(C._object_type(data))
		stringRepr := stringRepresentation(data)
		err = fmt.Errorf("Unsupported type %s: %s provided as data by %s", typeName, stringRepr, checkID)
		return
	}

	keys := C.PyDict_Keys(data)
	if keys == nil {
		pyErr, err := glock.getPythonError()

		if err != nil {
			return nil, fmt.Errorf("An error occurred while reading python data: %v", err)
		}
		return nil, errors.New(pyErr)
	}
	defer C.Py_DecRef(keys)

	newData := make(map[string]interface{})
	var entry *C.PyObject
	for i := 0; i < python.PyList_GET_SIZE(python.PyObject_FromVoidPtr(unsafe.Pointer(keys))); i++ {
		entryName := C.PyString_AsString(C.PyList_GetItem(keys, C.Py_ssize_t(i)))
		entry = C.PyDict_GetItemString(data, entryName)
		rvalue, rerr := extractStructureValueFrom(entry, checkID)
		if rerr != nil {
			return nil, rerr
		} else {
			newData[C.GoString(entryName)] = rvalue
		}
	}

	_data = newData

	return _data, nil
}

func extractStructureValueFrom(value *C.PyObject, checkID string) (_value interface{}, err error) {
	if isNone(value) {
		return make(map[string]interface{}), nil
	} else if int(C._PyString_Check(value)) != 0 {
		return C.GoString(C.PyString_AsString(value)), nil
	} else if int(C._PyInt_Check(value)) != 0 {
		return int64(C.PyInt_AsLong(value)), nil
	} else if int(C._PyDict_Check(value)) != 0 {
		return extractStructureFromObject(value, checkID)
	} else if int(C.PySequence_Check(value)) != 0 {
		errMsg := C.CString("expected value to be a sequence")
		defer C.free(unsafe.Pointer(errMsg))

		var seq *C.PyObject
		seq = C.PySequence_Fast(value, errMsg) // seq is a new reference, has to be decref'd
		if seq == nil {
			err = errors.New("can't iterate on sequence")
			return
		}
		defer C.Py_DecRef(seq)

		var i C.Py_ssize_t
		size := C.PySequence_Fast_Get_Size(seq)
		data := make([]interface{}, 0)
		for i = 0; i < size; i++ {
			item := C.PySequence_Fast_Get_Item(seq, i) // `item` is borrowed, no need to decref
			extracted, rerr := extractStructureValueFrom(item, checkID)
			if rerr != nil {
				return nil, rerr
			} else {
				data = append(data, extracted)
			}
		}

		return data, nil
	} else {
		typeName := C.GoString(C._object_type(value))
		stringRepr := stringRepresentation(value)
		return nil, fmt.Errorf("Unsupported type %s: %s in data provided by %s", typeName, stringRepr, checkID)
	}
}

func initStackState() {
	C.initstackstate()
}
