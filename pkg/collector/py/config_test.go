// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build cpython

// NOTICE: See TestMain function in `utils_test.go` for Python initialization
package py

import (
	"io/ioutil"
	"reflect"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/integration"
	"github.com/mitchellh/reflectwalk"
	"github.com/sbinet/go-python"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gopkg.in/yaml.v2"
)

// Check that the result of the func is the same as the dict contained in the test directory,
// computed with PyYAML
func TestToPython(t *testing.T) {
	yamlFile, err := ioutil.ReadFile("tests/complex.yaml")
	if err != nil {
		t.Fatalf("Expected empty error message, found: %s", err)
	}

	c := make(integration.RawMap)

	err = yaml.Unmarshal(yamlFile, &c)
	if err != nil {
		t.Fatalf("Expected empty error message, found: %s", err)
	}

	res, err := ToPython(&c)
	if err != nil {
		t.Fatalf("Expected empty error message, found: %s", err)
	}

	// Lock the GIL and release it at the end
	gstate := newStickyLock()
	defer gstate.unlock()

	if !python.PyDict_Check(res) {
		t.Fatalf("Result is not a Python dict")
	}

	m := python.PyImport_ImportModuleNoBlock("tests.complex")
	if m == nil {
		t.Fatalf("Unable to import module complex.py")
	}

	d := m.GetAttrString("d")
	if d == nil {
		t.Fatalf("Unable to import test dictionary from module complex.py")
	}

	same := res.RichCompareBool(d, python.Py_EQ)
	if same < 1 {
		t.Fatalf("Result and template dict must be the same:\n Result: %s\n Template: %s",
			python.PyString_AsString(res.Str()),
			python.PyString_AsString(d.Str()))
	}
}

func TestToPythonInt(t *testing.T) {
	var a int
	var b int32
	a = 21
	b = 32

	res, err := ToPython(&a)
	require.Nil(t, err, "Expected empty error message")

	// Lock the GIL and release it at the end
	gstate := newStickyLock()
	defer gstate.unlock()

	assert.True(t, python.PyInt_Check(res), "Result is not a Python dict")

	res, err = ToPython(&b)
	require.Nil(t, err, "Expected empty error message")
	assert.True(t, python.PyInt_Check(res), "Result is not a Python dict")
}

func TestToPythonfloat(t *testing.T) {
	var a float64
	var b float32
	a = 64.0
	b = 32.0

	res, err := ToPython(&a)
	require.Nil(t, err, "Expected empty error message")

	// Lock the GIL and release it at the end
	gstate := newStickyLock()
	defer gstate.unlock()

	assert.True(t, python.PyFloat_Check(res), "Result is not a Python float")

	res, err = ToPython(&b)
	require.Nil(t, err, "Expected empty error message")
	assert.True(t, python.PyFloat_Check(res), "Result is not a Python float")
}

func TestToPythonBool(t *testing.T) {
	var a bool
	a = true

	res, err := ToPython(&a)
	require.Nil(t, err, "Expected empty error message")

	// Lock the GIL and release it at the end
	gstate := newStickyLock()
	defer gstate.unlock()

	assert.True(t, python.PyBool_Check(res), "Result is not a Python bool")
}

func TestToPythonString(t *testing.T) {
	var a string
	a = "this is a test string"

	res, err := ToPython(&a)
	require.Nil(t, err, "Expected empty error message")

	// Lock the GIL and release it at the end
	gstate := newStickyLock()
	defer gstate.unlock()

	assert.True(t, python.PyString_Check(res), "Result is not a Python string")
}

func TestToPythonDuration(t *testing.T) {
	var a time.Duration
	a = 1 * time.Second

	res, err := ToPython(&a)
	require.Nil(t, err, "Expected empty error message")

	// Lock the GIL and release it at the end
	gstate := newStickyLock()
	defer gstate.unlock()

	assert.True(t, python.PyInt_Check(res), "Result is not a Python string")
}

func TestWalkerPush(t *testing.T) {
	gstate := newStickyLock()
	defer gstate.unlock()

	w := new(walker)
	d := python.PyDict_New()

	// initial state
	w.push(d)
	if len(w.containersStack) != 1 {
		t.Fatalf("Stack should have size 1, found: %d", len(w.containersStack))
	}
	if w.currentContainer != w.containersStack[0] {
		t.Fatalf("Initial state not consistent, found %v", w)
	}
	if w.currentContainer != d {
		t.Fatalf("Initial state not consistent, found %v", w)
	}

	// push again
	d2 := python.PyDict_New()
	w.push(d2)
	if len(w.containersStack) != 2 {
		t.Fatalf("Stack should have size 2, found: %d", len(w.containersStack))
	}
	if w.currentContainer != d2 {
		t.Fatalf("Initial state not consistent, found %v", w)
	}

	// and again
	l := python.PyList_New(0)
	w.push(l)
	if len(w.containersStack) != 3 {
		t.Fatalf("Stack should have size 3, found: %d", len(w.containersStack))
	}
	if w.currentContainer != l {
		t.Fatalf("Initial state not consistent, found %v", w)
	}
}

func TestWalkerPop(t *testing.T) {
	gstate := newStickyLock()
	defer gstate.unlock()

	w := new(walker)
	d := python.PyDict_New()
	w.push(d)
	w.push(python.PyDict_New())
	w.pop()
	if len(w.containersStack) != 1 {
		t.Fatalf("Stack should have size 1, found: %d", len(w.containersStack))
	}
	if w.currentContainer != d {
		t.Fatalf("Initial state not consistent, found %v", w)
	}

	// empty stack
	w.pop()

	// try to pop from an empty stack
	w.pop()
	if w.currentContainer != d {
		t.Fatalf("Initial state not consistent, found %v", w)
	}
}

func TestWalkerEnter(t *testing.T) {
	w := new(walker)
	w.Enter(reflectwalk.Map)
	if len(w.containersStack) != 1 {
		t.Fatalf("Stack should have size 1, found: %d", len(w.containersStack))
	}
	if !python.PyDict_Check(w.currentContainer) {
		t.Fatalf("Current container is not a dict")
	}

	w.Enter(reflectwalk.Slice)
	if len(w.containersStack) != 2 {
		t.Fatalf("Stack should have size 2, found: %d", len(w.containersStack))
	}
	if python.PyDict_Check(w.currentContainer) {
		t.Fatalf("Current container is a dict, shouldnt be")
	}
}

func TestWalkerExit(t *testing.T) {
	w := new(walker)
	w.lastKey = "foo"
	w.Exit(reflectwalk.Map)
	if len(w.containersStack) != 0 {
		t.Fatalf("Stack should have size 0, found: %d", len(w.containersStack))
	}
	if w.lastKey != "" {
		t.Fatalf("Last key should be empty, found: %s", w.lastKey)
	}

	w = new(walker)
	w.lastKey = "bar"
	w.Exit(reflectwalk.Map)
	if len(w.containersStack) != 0 {
		t.Fatalf("Stack should have size 0, found: %d", len(w.containersStack))
	}
	if w.lastKey != "" {
		t.Fatalf("Last key should be empty, found: %s", w.lastKey)
	}

	// fill the stack to test cleanup procedure
	gstate := newStickyLock()
	w.push(python.PyDict_New())
	gstate.unlock()
	w.Exit(reflectwalk.WalkLoc)
	if w.containersStack != nil {
		t.Fatalf("Stack should be nil, found: %v", w.containersStack)
	}
	if w.currentContainer != nil {
		t.Fatalf("Current container should be nil, found: %v", w.currentContainer)
	}
}

func TestIfToPy(t *testing.T) {
	// ifToPy is supposed to be invoked holding the GIL
	gstate := newStickyLock()
	defer gstate.unlock()

	var i interface{}

	i = 42
	val := ifToPy(reflect.ValueOf(i))
	if !python.PyInt_Check(val) {
		t.Fatalf("Return value is not int")
	}

	i = "Snafu"
	val = ifToPy(reflect.ValueOf(i))
	if !python.PyString_Check(val) {
		t.Fatalf("Return value is not a string")
	}

	i = false
	val = ifToPy(reflect.ValueOf(i))
	if !python.PyBool_Check(val) {
		t.Fatalf("Return value is not bool")
	}
}

func TestMapElem(t *testing.T) {
	k := "foo"
	v := "bar"
	m := map[string]string{}
	w := new(walker)
	w.currentContainer = python.PyDict_New()
	w.MapElem(reflect.ValueOf(m), reflect.ValueOf(k), reflect.ValueOf(v))
	if w.lastKey != k {
		t.Fatalf("Expected key value foo, found %s", w.lastKey)
	}

	pkey := python.PyString_FromString(w.lastKey)
	ok, _ := python.PyDict_Contains(w.currentContainer, pkey)
	if !ok {
		t.Fatalf("Key not found in dictionary")
	}
}

func TestSliceElem(t *testing.T) {
	v := "bar"
	w := new(walker)
	w.currentContainer = python.PyList_New(0)
	w.SliceElem(0, reflect.ValueOf(v))
	l := python.PyList_Size(w.currentContainer)
	if l != 1 {
		t.Fatalf("Expected list length 1, found %d", l)
	}
}
