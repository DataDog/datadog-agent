// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python && test

package python

import (
	"runtime"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"

	"github.com/stretchr/testify/assert"
)

/*
#include <datadog_agent_rtloader.h>
#include <string.h>

int get_class_calls = 0;
int get_class_return = 0;
int get_class_dd_wheel_return = 0;
const char *get_class_name = NULL;
const char *get_class_dd_wheel_name = NULL;
rtloader_pyobject_t *get_class_py_module = NULL;
rtloader_pyobject_t *get_class_py_class = NULL;
rtloader_pyobject_t *get_class_dd_wheel_py_module = NULL;
rtloader_pyobject_t *get_class_dd_wheel_py_class = NULL;

int get_class(rtloader_t *rtloader, const char *name, rtloader_pyobject_t **py_module, rtloader_pyobject_t **py_class) {

	get_class_calls++;

	// check if we're loading a dd Wheel
	if (strncmp(name, "datadog_checks.", 15) == 0) {
		get_class_dd_wheel_name = name;
		*py_module = get_class_dd_wheel_py_module;
		*py_class = get_class_dd_wheel_py_class;
		return get_class_dd_wheel_return;
	}

	get_class_name = name;
	*py_module = get_class_py_module;
	*py_class = get_class_py_class;
	return get_class_return;
}

int get_attr_string_return = 0;
rtloader_pyobject_t *get_attr_string_py_class = NULL;
const char *get_attr_string_attr_name = NULL;
char *get_attr_string_attr_value = NULL;

int get_attr_string(rtloader_t *rtloader, rtloader_pyobject_t *py_class, const char *attr_name, char **value) {
	get_attr_string_py_class = py_class;
	get_attr_string_attr_name = strdup(attr_name);
	*value = get_attr_string_attr_value;

	return get_attr_string_return;
}

void reset_loader_mock() {
	get_class_calls = 0;
	get_class_return = 0;
	get_class_dd_wheel_return = 0;
	get_class_name = NULL;
	get_class_dd_wheel_name = NULL;
	get_class_py_module = NULL;
	get_class_py_class = NULL;
	get_class_dd_wheel_py_module = NULL;
	get_class_dd_wheel_py_class = NULL;

	get_attr_string_return = 0;
	get_attr_string_py_class = NULL;
	get_attr_string_attr_name = NULL;
	get_attr_string_attr_value = NULL;
}
*/
import "C"

func testLoadCustomCheck(t *testing.T) {
	C.reset_loader_mock()

	conf := integration.Config{
		Name:       "fake_check",
		Instances:  []integration.Data{integration.Data("{\"value\": 1}")},
		InitConfig: integration.Data("{}"),
	}

	rtloader = newMockRtLoaderPtr()
	defer func() { rtloader = nil }()

	loader, err := NewPythonCheckLoader()
	assert.Nil(t, err)

	// testing loading custom checks
	C.get_class_return = 1
	C.get_class_py_module = newMockPyObjectPtr()
	C.get_class_py_class = newMockPyObjectPtr()
	C.get_attr_string_return = 0

	check, err := loader.Load(conf, conf.Instances[0])
	// Remove check finalizer that may trigger race condition while testing
	runtime.SetFinalizer(check, nil)

	assert.Nil(t, err)
	assert.Equal(t, "fake_check", check.(*PythonCheck).ModuleName)
	assert.Equal(t, "unversioned", check.(*PythonCheck).version)
	assert.Equal(t, C.get_class_py_class, check.(*PythonCheck).class)
	// test we call get_attr_string on the module
	assert.Equal(t, C.get_attr_string_py_class, C.get_class_py_module)
}

func testLoadWheelCheck(t *testing.T) {
	C.reset_loader_mock()

	conf := integration.Config{
		Name:       "fake_check",
		Instances:  []integration.Data{integration.Data("{\"value\": 1}")},
		InitConfig: integration.Data("{}"),
	}

	rtloader = newMockRtLoaderPtr()
	defer func() { rtloader = nil }()

	loader, err := NewPythonCheckLoader()
	assert.Nil(t, err)

	// testing loading dd wheels
	C.get_class_dd_wheel_return = 1
	C.get_class_dd_wheel_py_module = newMockPyObjectPtr()
	C.get_class_dd_wheel_py_class = newMockPyObjectPtr()
	C.get_attr_string_return = 1
	C.get_attr_string_attr_value = C.CString("1.2.3")

	check, err := loader.Load(conf, conf.Instances[0])
	// Remove check finalizer that may trigger race condition while testing
	runtime.SetFinalizer(check, nil)

	assert.Nil(t, err)
	assert.Equal(t, "fake_check", check.(*PythonCheck).ModuleName)
	assert.Equal(t, "1.2.3", check.(*PythonCheck).version)
	assert.Equal(t, C.get_class_dd_wheel_py_class, check.(*PythonCheck).class)
	// test we call get_attr_string on the module
	assert.Equal(t, C.get_attr_string_py_class, C.get_class_dd_wheel_py_module)
}
