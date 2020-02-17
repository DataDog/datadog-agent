// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build python,test

package python

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
#include <datadog_agent_rtloader.h>
#include <stdlib.h>

int gil_locked_calls = 0;
rtloader_gilstate_t ensure_gil(rtloader_t *s) {
	gil_locked_calls++;
	return 0;
}

int gil_unlocked_calls = 0;
void release_gil(rtloader_t *s, rtloader_gilstate_t state) {
	gil_unlocked_calls++;
}

int rtloader_incref_calls = 0;
void rtloader_incref(rtloader_t *s, rtloader_pyobject_t *p) {
	rtloader_incref_calls++;
	return;
}

int rtloader_decref_calls = 0;
void rtloader_decref(rtloader_t *s, rtloader_pyobject_t *p) {
	rtloader_decref_calls++;
	return;
}

char **get_checks_warnings_return = NULL;
int get_checks_warnings_calls = 0;
char **get_checks_warnings(rtloader_t *s, rtloader_pyobject_t *check) {
	get_checks_warnings_calls++;
	return get_checks_warnings_return;
}

int has_error_calls = 0;
int has_error_return = 0;
int has_error(const rtloader_t *s) {
	has_error_calls++;
	return has_error_return;
}

int get_error_calls = 0;
char *get_error_return = "";
const char *get_error(const rtloader_t *s) {
	get_error_calls++;
	return get_error_return;
}

int rtloader_free_calls = 0;
void rtloader_free(rtloader_t *s, void *p) {
	rtloader_free_calls++;
	return;
}

int run_check_calls = 0;
char *run_check_return = NULL;
rtloader_pyobject_t *run_check_instance = NULL;
char *run_check(rtloader_t *s, rtloader_pyobject_t *check) {
	run_check_instance = check;
	run_check_calls++;
	return run_check_return;
}

//
// get_check MOCK
//

int get_check_return = 0;
int get_check_calls = 0;
rtloader_pyobject_t *get_check_py_class = NULL;
const char *get_check_init_config = NULL;
const char *get_check_instance = NULL;
const char *get_check_check_id = NULL;
const char *get_check_check_name = NULL;
rtloader_pyobject_t *get_check_check = NULL;

int get_check(rtloader_t *rtloader, rtloader_pyobject_t *py_class, const char *init_config, const char *instance,
const char *check_id, const char *check_name, rtloader_pyobject_t **check) {

	get_check_py_class = py_class;
	get_check_init_config = strdup(init_config);
	get_check_instance = strdup(instance);
	get_check_check_id = strdup(check_id);
	get_check_check_name = strdup(check_name);
	*check = get_check_check;

	get_check_calls++;
	return get_check_return;
}

// get_check_deprecated MOCK

int get_check_deprecated_calls = 0;
int get_check_deprecated_return = 0;
rtloader_pyobject_t *get_check_deprecated_py_class = NULL;
const char *get_check_deprecated_init_config = NULL;
const char *get_check_deprecated_instance = NULL;
const char *get_check_deprecated_check_id = NULL;
const char *get_check_deprecated_check_name = NULL;
const char *get_check_deprecated_agent_config = NULL;
rtloader_pyobject_t *get_check_deprecated_check = NULL;

int get_check_deprecated(rtloader_t *rtloader, rtloader_pyobject_t *py_class, const char *init_config,
const char *instance, const char *agent_config, const char *check_id, const char *check_name,
rtloader_pyobject_t **check) {

	get_check_deprecated_py_class = py_class;
	get_check_deprecated_init_config = strdup(init_config);
	get_check_deprecated_instance = strdup(instance);
	get_check_deprecated_check_id = strdup(check_id);
	get_check_deprecated_check_name = strdup(check_name);
	get_check_deprecated_agent_config = strdup(agent_config);
	*check = get_check_deprecated_check;

	get_check_deprecated_calls++;
	return get_check_deprecated_return;
}

void reset_check_mock() {
	gil_locked_calls = 0;
	gil_unlocked_calls = 0;
	rtloader_incref_calls = 0;
	rtloader_decref_calls = 0;
	get_checks_warnings_return = NULL;
	get_checks_warnings_calls = 0;
	has_error_calls = 0;
	has_error_return = 0;
	get_error_calls = 0;
	get_error_return = "";
	rtloader_free_calls = 0;
	run_check_calls = 0;
	get_check_return = 0;

	get_check_return = 0;
	get_check_calls = 0;
	get_check_py_class = NULL;
	get_check_init_config = NULL;
	get_check_instance = NULL;
	get_check_check_id = NULL;
	get_check_check_name = NULL;
	get_check_check = NULL;

	get_check_deprecated_calls = 0;
	get_check_deprecated_return = 0;
	get_check_deprecated_py_class = NULL;
	get_check_deprecated_init_config = NULL;
	get_check_deprecated_instance = NULL;
	get_check_deprecated_check_id = NULL;
	get_check_deprecated_check_name = NULL;
	get_check_deprecated_agent_config = NULL;
	get_check_deprecated_check = NULL;

}
*/
import "C"

func testRunCheck(t *testing.T) {
	check := NewPythonCheck("fake_check", nil)
	check.instance = &C.rtloader_pyobject_t{}

	C.reset_check_mock()
	C.run_check_return = C.CString("")
	warn := []*C.char{C.CString("warn1"), C.CString("warn2"), nil}
	C.get_checks_warnings_return = &warn[0]

	err := check.runCheck(false)
	assert.Nil(t, err)

	assert.Equal(t, C.int(1), C.gil_locked_calls)
	assert.Equal(t, C.int(1), C.gil_unlocked_calls)
	assert.Equal(t, C.int(1), C.run_check_calls)
	assert.Equal(t, C.int(1), C.get_checks_warnings_calls)

	assert.Equal(t, check.instance, C.run_check_instance)
	assert.Equal(t, check.lastWarnings, []error{fmt.Errorf("warn1"), fmt.Errorf("warn2")})
}

func testRunErrorNil(t *testing.T) {
	check := NewPythonCheck("fake_check", nil)
	check.instance = &C.rtloader_pyobject_t{}

	C.reset_check_mock()
	C.run_check_return = nil
	C.has_error_return = 1
	C.get_error_return = C.CString("some error")

	err := check.runCheck(false)
	assert.NotNil(t, err)
	assert.NotNil(t, fmt.Errorf("some error"), err)

	assert.Equal(t, C.int(1), C.gil_locked_calls)
	assert.Equal(t, C.int(1), C.gil_unlocked_calls)
	assert.Equal(t, C.int(1), C.run_check_calls)
	assert.Equal(t, C.int(0), C.get_checks_warnings_calls)

	assert.Equal(t, check.instance, C.run_check_instance)
}

func testRunErrorReturn(t *testing.T) {
	check := NewPythonCheck("fake_check", nil)
	check.instance = &C.rtloader_pyobject_t{}

	C.reset_check_mock()
	C.run_check_return = C.CString("not OK")

	err := check.runCheck(false)
	assert.NotNil(t, err)
	assert.NotNil(t, fmt.Errorf("not OK"), err)

	assert.Equal(t, C.int(1), C.gil_locked_calls)
	assert.Equal(t, C.int(1), C.gil_unlocked_calls)
	assert.Equal(t, C.int(1), C.run_check_calls)
	assert.Equal(t, C.int(1), C.get_checks_warnings_calls)

	assert.Equal(t, check.instance, C.run_check_instance)
}

func testRun(t *testing.T) {
	sender := mocksender.NewMockSender(check.ID("testID"))
	sender.SetupAcceptAll()

	c := NewPythonCheck("fake_check", nil)
	c.instance = &C.rtloader_pyobject_t{}
	c.id = check.ID("testID")

	C.reset_check_mock()
	C.run_check_return = C.CString("")

	err := c.Run()
	assert.Nil(t, err)

	assert.Equal(t, C.int(1), C.gil_locked_calls)
	assert.Equal(t, C.int(1), C.gil_unlocked_calls)
	assert.Equal(t, C.int(1), C.run_check_calls)
	assert.Equal(t, C.int(1), C.has_error_calls)
	assert.Equal(t, C.int(0), C.get_error_calls)
	assert.Equal(t, C.int(1), C.get_checks_warnings_calls)

	assert.Equal(t, c.instance, C.run_check_instance)

	sender.Mock.AssertCalled(t, "Commit")
}

func testRunSimple(t *testing.T) {
	sender := mocksender.NewMockSender(check.ID("testID"))
	sender.SetupAcceptAll()

	c := NewPythonCheck("fake_check", nil)
	c.instance = &C.rtloader_pyobject_t{}
	c.id = check.ID("testID")

	C.reset_check_mock()
	C.run_check_return = C.CString("")

	err := c.RunSimple()
	assert.Nil(t, err)

	assert.Equal(t, C.int(1), C.gil_locked_calls)
	assert.Equal(t, C.int(1), C.gil_unlocked_calls)
	assert.Equal(t, C.int(1), C.run_check_calls)
	assert.Equal(t, C.int(1), C.has_error_calls)
	assert.Equal(t, C.int(0), C.get_error_calls)
	assert.Equal(t, C.int(1), C.get_checks_warnings_calls)

	assert.Equal(t, c.instance, C.run_check_instance)

	sender.Mock.AssertNotCalled(t, "commit")
}

func testConfigure(t *testing.T) {
	c := NewPythonCheck("fake_check", nil)
	c.class = &C.rtloader_pyobject_t{}

	C.reset_check_mock()

	C.get_check_return = 1
	C.get_check_check = &C.rtloader_pyobject_t{}
	err := c.Configure(integration.Data("{\"val\": 21}"), integration.Data("{\"val\": 21}"), "test")
	assert.Nil(t, err)

	assert.Equal(t, c.class, C.get_check_py_class)
	assert.Equal(t, "{\"val\": 21}", C.GoString(C.get_check_init_config))
	assert.Equal(t, "{\"val\": 21}", C.GoString(C.get_check_instance))
	assert.Equal(t, string(c.id), C.GoString(C.get_check_check_id))
	assert.Equal(t, "fake_check", C.GoString(C.get_check_check_name))
	assert.Equal(t, C.get_check_check, c.instance)

	assert.Nil(t, C.get_check_deprecated_py_class)
	assert.Nil(t, C.get_check_deprecated_init_config)
	assert.Nil(t, C.get_check_deprecated_instance)
	assert.Nil(t, C.get_check_deprecated_check_id)
	assert.Nil(t, C.get_check_deprecated_check_name)
	assert.Nil(t, C.get_check_deprecated_agent_config)
	assert.Nil(t, C.get_check_deprecated_check)
}

func testConfigureDeprecated(t *testing.T) {
	c := NewPythonCheck("fake_check", nil)
	c.class = &C.rtloader_pyobject_t{}

	C.reset_check_mock()

	C.get_check_return = 0
	C.get_check_deprecated_check = &C.rtloader_pyobject_t{}
	C.get_check_deprecated_return = 1
	err := c.Configure(integration.Data("{\"val\": 21}"), integration.Data("{\"val\": 21}"), "test")
	assert.Nil(t, err)

	assert.Equal(t, c.class, C.get_check_py_class)
	assert.Equal(t, "{\"val\": 21}", C.GoString(C.get_check_init_config))
	assert.Equal(t, "{\"val\": 21}", C.GoString(C.get_check_instance))
	assert.Equal(t, string(c.id), C.GoString(C.get_check_check_id))
	assert.Equal(t, "fake_check", C.GoString(C.get_check_check_name))
	assert.Nil(t, C.get_check_check)

	assert.Equal(t, c.class, C.get_check_deprecated_py_class)
	assert.Equal(t, "{\"val\": 21}", C.GoString(C.get_check_deprecated_init_config))
	assert.Equal(t, "{\"val\": 21}", C.GoString(C.get_check_deprecated_instance))
	assert.Equal(t, string(c.id), C.GoString(C.get_check_deprecated_check_id))
	assert.Equal(t, "fake_check", C.GoString(C.get_check_deprecated_check_name))
	require.NotNil(t, C.get_check_deprecated_agent_config)
	assert.NotEqual(t, "", C.GoString(C.get_check_deprecated_agent_config))
	assert.Equal(t, c.instance, C.get_check_deprecated_check)
}
