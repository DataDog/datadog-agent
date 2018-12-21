// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build cpython

package py

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	python "github.com/DataDog/go-python3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
)

// Setup the test module
func TestMain(m *testing.M) {
	rootDir := "."
	testsDir := "tests"
	distDir := "../../../cmd/agent/dist"

	// best effort for abs path
	if _, fileName, _, ok := runtime.Caller(0); ok {
		rootDir = filepath.Dir(fileName)
		testsDir = filepath.Join(rootDir, testsDir)
		distDir = filepath.Join(rootDir, distDir)
	}
	state := Initialize(rootDir, testsDir, distDir)

	// testing this package needs an inited aggregator
	// to work properly.
	aggregator.InitAggregatorWithFlushInterval(nil, "", "", time.Hour)

	ret := m.Run()

	python.PyEval_RestoreThread(state)
	// benchmarks don't like python.Finalize() for some reason, let's just not call it

	os.Exit(ret)
}

func TestFindSubclassOf(t *testing.T) {
	gstate := newStickyLock()
	defer gstate.unlock()

	fooModule := python.PyImport_ImportModule("foo")
	fooClass := fooModule.GetAttrString("Foo")
	barModule := python.PyImport_ImportModule("bar")
	barClass := barModule.GetAttrString("Bar")

	// invalid input
	sclass, err := findSubclassOf(nil, nil, gstate)
	assert.NotNil(t, err)

	// pass something that's not a Type
	sclass, err = findSubclassOf(python.PyTuple_New(0), fooModule, gstate)
	assert.NotNil(t, err)
	sclass, err = findSubclassOf(fooClass, python.PyTuple_New(0), gstate)
	assert.NotNil(t, err)

	// Foo in foo module, only Foo itself found
	sclass, err = findSubclassOf(fooClass, fooModule, gstate)
	assert.NotNil(t, err)

	// Bar in foo module, no class found
	sclass, err = findSubclassOf(barClass, fooModule, gstate)
	assert.NotNil(t, err)

	// Foo in bar module, get Bar
	sclass, err = findSubclassOf(fooClass, barModule, gstate)
	require.Nil(t, err)
	assert.Equal(t, 1, sclass.RichCompareBool(barClass, python.Py_EQ))

	// Multiple inheritance test
	multiBaseModule := python.PyImport_ImportModule("testcheck_multi_base")
	baseCheckClass := multiBaseModule.GetAttrString("BaseClass")
	multiModule := python.PyImport_ImportModule("testcheck_multi")
	derivedCheckClass := multiModule.GetAttrString("DerivedCheck")
	sclass, err = findSubclassOf(baseCheckClass, multiModule, gstate)
	require.Nil(t, err)
	assert.Equal(t, 1, sclass.RichCompareBool(derivedCheckClass, python.Py_EQ))
}

func TestSubprocessBindings(t *testing.T) {
	gstate := newStickyLock()
	defer gstate.unlock()

	utilModule := python.PyImport_ImportModule("_util")
	assert.NotNil(t, utilModule)
	defer utilModule.DecRef()

	getSubprocessOutput := utilModule.GetAttrString("get_subprocess_output")
	assert.NotNil(t, getSubprocessOutput)
	defer getSubprocessOutput.DecRef()

	// This call will go: python interpreter -> C-binding -> go-lang and back
	args := python.PyTuple_New(2)
	kwargs := python.PyDict_New()
	defer args.DecRef()
	defer kwargs.DecRef()

	cmdList := python.PyList_New(0)
	defer cmdList.DecRef()
	cmd := python.PyUnicode_FromString("ls")
	defer cmd.DecRef()
	arg := python.PyUnicode_FromString("-l")
	defer arg.DecRef()

	err := python.PyList_Insert(cmdList, 0, cmd)
	assert.Zero(t, err)
	err = python.PyList_Insert(cmdList, 1, arg)
	assert.Zero(t, err)

	raise := python.PyBool_FromLong(1)
	assert.NotNil(t, raise)

	python.PyTuple_SetItem(args, 0, cmdList)
	python.PyTuple_SetItem(args, 1, raise)

	res := getSubprocessOutput.Call(args, kwargs)
	assert.NotNil(t, res)
	assert.True(t, python.PyTuple_Check(res))

	if runtime.GOOS != "windows" {
		exc := python.PyErr_Occurred()
		assert.Nil(t, exc)

		assert.NotEqual(t, res, python.Py_None)

		// stdout
		assert.True(t, python.PyTuple_Check(res))
		pyOutput := python.PyTuple_GetItem(res, 0)
		assert.NotNil(t, pyOutput)
		output := python.PyUnicode_AsUTF8(pyOutput)
		assert.NotZero(t, len(output))
		t.Logf("command output was: %v", output)

		// Return Code
		retcode := python.PyTuple_GetItem(res, 2)
		assert.NotNil(t, retcode)
		assert.Zero(t, python.PyLong_AsLong(retcode))
	}
}

func TestGetModuleName(t *testing.T) {
	name := getModuleName("foo.bar.baz")
	if name != "baz" {
		t.Fatalf("Expected baz, found: %s", name)
	}

	name = getModuleName("baz")
	if name != "baz" {
		t.Fatalf("Expected baz, found: %s", name)
	}

	name = getModuleName("")
	if name != "" {
		t.Fatalf("Expected empty string, found: %s", name)
	}
}
