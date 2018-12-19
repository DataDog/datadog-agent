// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build cpython

package py

import (
	"reflect"
	"time"

	python "github.com/DataDog/go-python3"
	"github.com/mitchellh/reflectwalk"
)

// we use this struct to walk through YAML results and convert them to Python stuff
type walker struct {
	result           *python.PyObject
	lastKey          string
	containersStack  []*python.PyObject
	currentContainer *python.PyObject
}

// ToPython converts a go object into a Python object
func ToPython(obj interface{}) (*python.PyObject, error) {
	w := new(walker)
	err := reflectwalk.Walk(obj, w)

	return w.result, err
}

// Primitive convert a basic type to python (int, bool, string, ...)
func (w *walker) Primitive(v reflect.Value) error {
	// if we are currently in a map or slice context: do nothing
	if w.currentContainer != nil {
		return nil
	}

	// if not: we are converting a simple type
	gstate := newStickyLock()
	defer gstate.unlock()

	w.result = ifToPy(v)
	return nil
}

// push the old container to the stack and start using the new one
// Notice: the GIL must be acquired before calling this method
// push steals the reference to the passed *PyObject
func (w *walker) push(newc *python.PyObject) {
	// special case: init
	if w.result == nil {
		w.containersStack = append(w.containersStack, newc)
		w.currentContainer = newc
		w.result = newc
		newc.IncRef() // the first assignment steals the ref, we need to IncRef for the 2nd assignment
		return
	}

	// add new container to the current one before pushing it to the stack
	// we can safely assume it's either a `list` or a `dict`
	if python.PyDict_Check(w.currentContainer) {
		k := python.PyUnicode_FromString(w.lastKey)
		defer k.DecRef()
		python.PyDict_SetItem(w.currentContainer, k, newc) // steal the ref, no IncRef
	} else {
		python.PyList_Append(w.currentContainer, newc) // steal the ref, no IncRef
	}

	// push it like there's no tomorrow
	w.containersStack = append(w.containersStack, w.currentContainer)
	w.currentContainer = newc
}

// pop an old container and start adding new stuff to it
// do nothing if the stack is empty
func (w *walker) pop() {
	l := len(w.containersStack)
	if l > 0 {
		w.currentContainer.DecRef()
		w.currentContainer = w.containersStack[l-1]
		w.containersStack = w.containersStack[:l-1]
	}
}

// the walker is about to enter a new type, we only need to take action for
// Maps and Slices.
func (w *walker) Enter(l reflectwalk.Location) error {
	gstate := newStickyLock()
	defer gstate.unlock()

	switch l {
	case reflectwalk.Map:
		// push a new map on the stack
		w.push(python.PyDict_New())
	case reflectwalk.Slice:
		// push a new list on the stack
		w.push(python.PyList_New(0))
	}

	return nil
}

// the walker has done with the previous type, pop an old container
// from the stack and reset the last seen dict key
func (w *walker) Exit(l reflectwalk.Location) error {
	switch l {
	case reflectwalk.Map:
		fallthrough
	case reflectwalk.Slice:
		// Pop previous container
		w.lastKey = ""
		w.pop()
	case reflectwalk.WalkLoc:
		// Cleanup
		w.containersStack = nil
		w.currentContainer = nil
	}

	return nil
}

// only to implement Walker interface
func (w *walker) Map(m reflect.Value) error {
	return nil
}

// only to implement Walker interface
func (w *walker) Slice(s reflect.Value) error {
	return nil
}

// ugly but YAML returns only interfaces, need to introspect manually
// Notice: the GIL must be acquired before calling this method
func ifToPy(v reflect.Value) *python.PyObject {
	var pyval *python.PyObject
	vi := v.Interface()

	switch s := vi.(type) {
	case string:
		pyval = python.PyUnicode_FromString(s)
	case int:
		pyval = python.PyLong_FromLong(int(s))
	case int32:
		pyval = python.PyLong_FromLong(int(s))
	case int64:
		// This will only works on 64bit host. Since we don't offer 32bit build it's fine
		pyval = python.PyLong_FromLong(int(s))
	case time.Duration:
		// This will only works on 64bit host. Since we don't offer 32bit build it's fine
		pyval = python.PyLong_FromLong(int(s))
	case float32:
		pyval = python.PyFloat_FromDouble(float64(s))
	case float64:
		pyval = python.PyFloat_FromDouble(float64(s))
	case bool:
		if s {
			pyval = python.PyBool_FromLong(1)
		} else {
			pyval = python.PyBool_FromLong(0)
		}
	}

	return pyval
}

// go through map elements and convert to Python dict elements
func (w *walker) MapElem(m, k, v reflect.Value) error {
	// Lock the GIL and release it at the end
	gstate := newStickyLock()
	defer gstate.unlock()

	w.lastKey = k.Interface().(string)
	dictKey := python.PyUnicode_FromString(w.lastKey)
	defer dictKey.DecRef()

	// set the converted value in the Python dict
	pyval := ifToPy(v)
	if pyval != nil {
		defer pyval.DecRef()
		python.PyDict_SetItem(w.currentContainer, dictKey, pyval)
	}

	return nil
}

// go through slice items and convert to Python list items
func (w *walker) SliceElem(i int, v reflect.Value) error {
	// Lock the GIL and release it at the end
	gstate := newStickyLock()
	defer gstate.unlock()

	pyval := ifToPy(v)
	if pyval != nil {
		defer pyval.DecRef()
		python.PyList_Append(w.currentContainer, pyval)
	}

	return nil
}
