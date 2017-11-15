// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package py

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/sbinet/go-python"
	"github.com/stretchr/testify/assert"
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
	// to work properly
	aggregator.InitAggregator(nil, "")

	ret := m.Run()

	python.PyEval_RestoreThread(state)
	// benchmarks don't like python.Finalize() for some reason, let's just not call it

	os.Exit(ret)
}

func TestFindSubclassOf(t *testing.T) {
	gstate := newStickyLock()
	defer gstate.unlock()

	fooModule := python.PyImport_ImportModuleNoBlock("foo")
	fooClass := fooModule.GetAttrString("Foo")
	barModule := python.PyImport_ImportModuleNoBlock("bar")
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
	assert.Nil(t, err)
	assert.Equal(t, 1, sclass.RichCompareBool(barClass, python.Py_EQ))
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
