// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build python,test

package python

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
#include <datadog_agent_rtloader.h>

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

py_info_t *get_py_info(rtloader_t *sic) {
	py_info_t *i = malloc(sizeof(py_info_t));
	i->version = "fake python";
	i->path = "/fake/path";

	return i;
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
		Name: "fake_check",
		Instances: []integration.Data{integration.Data("{\"value\": 1}"),
			integration.Data("{\"value\": 2}")},
		InitConfig: integration.Data("{}"),
	}

	// init rtloader
	rtloader = &C.rtloader_t{}
	defer func() { rtloader = nil }()

	loader, err := NewPythonCheckLoader()
	assert.Nil(t, err)

	// testing loading custom checks
	C.get_class_return = 1
	C.get_class_py_module = &C.rtloader_pyobject_t{}
	C.get_class_py_class = &C.rtloader_pyobject_t{}
	C.get_attr_string_return = 0

	checks, err := loader.Load(conf)
	assert.Nil(t, err)
	require.Len(t, checks, 2)
	assert.Equal(t, "fake_check", checks[0].(*PythonCheck).ModuleName)
	assert.Equal(t, "fake_check", checks[1].(*PythonCheck).ModuleName)
	assert.Equal(t, "unversioned", checks[0].(*PythonCheck).version)
	assert.Equal(t, "unversioned", checks[1].(*PythonCheck).version)
	assert.Equal(t, C.get_class_py_class, checks[0].(*PythonCheck).class)
	assert.Equal(t, C.get_class_py_class, checks[1].(*PythonCheck).class)
	// test we call get_attr_string on the module
	assert.Equal(t, C.get_attr_string_py_class, C.get_class_py_module)
}

func testLoadWheelCheck(t *testing.T) {
	C.reset_loader_mock()

	conf := integration.Config{
		Name: "fake_check",
		Instances: []integration.Data{integration.Data("{\"value\": 1}"),
			integration.Data("{\"value\": 2}")},
		InitConfig: integration.Data("{}"),
	}

	// init rtloader
	rtloader = &C.rtloader_t{}
	defer func() { rtloader = nil }()

	loader, err := NewPythonCheckLoader()
	assert.Nil(t, err)

	// testing loading dd wheels
	C.get_class_dd_wheel_return = 1
	C.get_class_dd_wheel_py_module = &C.rtloader_pyobject_t{}
	C.get_class_dd_wheel_py_class = &C.rtloader_pyobject_t{}
	C.get_attr_string_return = 1
	C.get_attr_string_attr_value = C.CString("1.2.3")

	checks, err := loader.Load(conf)
	assert.Nil(t, err)
	require.Len(t, checks, 2)
	assert.Equal(t, "fake_check", checks[0].(*PythonCheck).ModuleName)
	assert.Equal(t, "fake_check", checks[1].(*PythonCheck).ModuleName)
	assert.Equal(t, "1.2.3", checks[0].(*PythonCheck).version)
	assert.Equal(t, "1.2.3", checks[1].(*PythonCheck).version)
	assert.Equal(t, C.get_class_dd_wheel_py_class, checks[0].(*PythonCheck).class)
	assert.Equal(t, C.get_class_dd_wheel_py_class, checks[1].(*PythonCheck).class)
	// test we call get_attr_string on the module
	assert.Equal(t, C.get_attr_string_py_class, C.get_class_dd_wheel_py_module)
}
