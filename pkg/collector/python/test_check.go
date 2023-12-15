// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python && test

package python

import (
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
)

/*
#include <datadog_agent_rtloader.h>
#include <stdlib.h>
#include <string.h>

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

int cancel_check_calls = 0;
rtloader_pyobject_t *cancel_check_instance = NULL;
void cancel_check(rtloader_t *s, rtloader_pyobject_t *check) {
	cancel_check_instance = check;
	cancel_check_calls++;
	return;
}

char *get_check_diagnoses_return = NULL;
int get_check_diagnoses_calls = 0;
char *get_check_diagnoses(rtloader_t *s, rtloader_pyobject_t *check) {
	get_check_diagnoses_calls++;
	return get_check_diagnoses_return;
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
	cancel_check_calls = 0;
	cancel_check_instance = NULL;

	get_check_deprecated_calls = 0;
	get_check_deprecated_return = 0;
	get_check_deprecated_py_class = NULL;
	get_check_deprecated_init_config = NULL;
	get_check_deprecated_instance = NULL;
	get_check_deprecated_check_id = NULL;
	get_check_deprecated_check_name = NULL;
	get_check_deprecated_agent_config = NULL;
	get_check_deprecated_check = NULL;

	get_check_diagnoses_return = NULL;
	get_check_diagnoses_calls = 0;
}
*/
import "C"

func testRunCheck(t *testing.T) {
	rtloader = newMockRtLoaderPtr()
	defer func() { rtloader = nil }()
	check, err := NewPythonFakeCheck(aggregator.NewNoOpSenderManager())
	if !assert.Nil(t, err) {
		return
	}

	check.instance = newMockPyObjectPtr()

	C.reset_check_mock()
	C.run_check_return = C.CString("")
	warn := []*C.char{C.CString("warn1"), C.CString("warn2"), nil}
	C.get_checks_warnings_return = &warn[0]

	err = check.runCheck(false)
	assert.Nil(t, err)

	assert.Equal(t, C.int(1), C.gil_locked_calls)
	assert.Equal(t, C.int(1), C.gil_unlocked_calls)
	assert.Equal(t, C.int(1), C.run_check_calls)
	assert.Equal(t, C.int(1), C.get_checks_warnings_calls)

	assert.Equal(t, check.instance, C.run_check_instance)
	assert.Equal(t, check.lastWarnings, []error{fmt.Errorf("warn1"), fmt.Errorf("warn2")})
}

func testRunCheckWithRuntimeNotInitializedError(t *testing.T) {
	rtloader = newMockRtLoaderPtr()
	defer func() { rtloader = nil }()
	check, err := NewPythonFakeCheck(aggregator.NewNoOpSenderManager())
	if !assert.Nil(t, err) {
		return
	}

	check.instance = newMockPyObjectPtr()

	C.reset_check_mock()
	C.run_check_return = C.CString("")

	rtloader = nil

	err = check.runCheck(false)
	assert.EqualError(
		t,
		err,
		"error acquiring the GIL: rtloader is not initialized",
	)
}

func testInitiCheckWithRuntimeNotInitialized(t *testing.T) {
	// Ensure RT pointer is zeroized
	rtloader = nil

	C.reset_check_mock()
	_, err := NewPythonFakeCheck(aggregator.NewNoOpSenderManager())
	if !assert.NotNil(t, err) {
		return
	}

	assert.EqualError(
		t,
		err,
		"error acquiring the GIL: rtloader is not initialized",
	)

	assert.Equal(t, C.int(0), C.gil_locked_calls)
	assert.Equal(t, C.int(0), C.gil_unlocked_calls)
	assert.Equal(t, C.int(0), C.run_check_calls)
	assert.Equal(t, C.int(0), C.rtloader_free_calls)
	assert.Equal(t, C.int(0), C.rtloader_incref_calls)
	assert.Equal(t, C.int(0), C.rtloader_decref_calls)
}

func testCheckCancel(t *testing.T) {
	rtloader = newMockRtLoaderPtr()
	defer func() { rtloader = nil }()
	check, err := NewPythonFakeCheck(aggregator.NewNoOpSenderManager())
	if !assert.Nil(t, err) {
		return
	}

	C.reset_check_mock()
	check.instance = newMockPyObjectPtr()
	C.run_check_return = C.CString("")

	err = check.runCheck(false)
	if !assert.Nil(t, err) {
		return
	}

	// Sanity checks to ensure known start state
	assert.Equal(t, C.int(1), C.gil_locked_calls)
	assert.Equal(t, C.int(1), C.gil_unlocked_calls)
	assert.Equal(t, C.int(0), C.rtloader_incref_calls)
	assert.Equal(t, C.int(0), C.rtloader_decref_calls)
	assert.Equal(t, C.int(0), C.cancel_check_calls)
	assert.Equal(t, check.instance, C.run_check_instance)

	check.Cancel()

	// Check that the lock was acquired
	assert.Equal(t, C.int(2), C.gil_locked_calls)
	assert.Equal(t, C.int(2), C.gil_unlocked_calls)

	// Check that the call was passed to C
	assert.Equal(t, C.int(1), C.cancel_check_calls)
	assert.Equal(t, check.instance, C.cancel_check_instance)
}

func testCheckCancelWhenRuntimeUnloaded(t *testing.T) {
	rtloader = newMockRtLoaderPtr()
	defer func() { rtloader = nil }()

	check, err := NewPythonFakeCheck(aggregator.NewNoOpSenderManager())
	if !assert.Nil(t, err) {
		return
	}

	C.reset_check_mock()
	check.instance = newMockPyObjectPtr()
	C.run_check_return = C.CString("")

	err = check.runCheck(false)
	if !assert.Nil(t, err) {
		return
	}

	// Sanity checks to ensure known start state
	assert.Equal(t, C.int(1), C.gil_locked_calls)
	assert.Equal(t, C.int(1), C.gil_unlocked_calls)
	assert.Equal(t, C.int(0), C.rtloader_incref_calls)
	assert.Equal(t, C.int(0), C.rtloader_decref_calls)
	assert.Equal(t, C.int(0), C.cancel_check_calls)
	assert.Equal(t, check.instance, C.run_check_instance)

	// Simulate rtloader being unloaded
	rtloader = nil

	check.Cancel()

	// There should be no invocations of cancel
	assert.Equal(t, C.int(0), C.cancel_check_calls)
}

func testFinalizer(t *testing.T) {
	rtloader = newMockRtLoaderPtr()
	defer func() {
		// We have to wrap this in locks otherwise the race detector complains
		pyDestroyLock.Lock()
		rtloader = nil
		pyDestroyLock.Unlock()
	}()

	check, err := NewPythonFakeCheck(aggregator.NewNoOpSenderManager())
	if !assert.Nil(t, err) {
		return
	}

	C.reset_check_mock()
	check.instance = newMockPyObjectPtr()
	C.run_check_return = C.CString("")

	err = check.runCheck(false)
	if !assert.Nil(t, err) {
		return
	}

	// Sanity checks to ensure known start state
	assert.Equal(t, C.int(1), C.gil_locked_calls)
	assert.Equal(t, C.int(1), C.gil_unlocked_calls)
	assert.Equal(t, C.int(0), C.rtloader_incref_calls)
	assert.Equal(t, C.int(0), C.rtloader_decref_calls)
	assert.Equal(t, check.instance, C.run_check_instance)

	pythonCheckFinalizer(check)

	// Finalizer runs in a goroutine so we have to wait a bit
	time.Sleep(100 * time.Millisecond)

	// Check that the lock was acquired
	assert.Equal(t, C.int(2), C.gil_locked_calls)
	assert.Equal(t, C.int(2), C.gil_unlocked_calls)

	// Check that the class and instance have been decref-d
	assert.Equal(t, C.int(0), C.rtloader_incref_calls)
	assert.Equal(t, C.int(2), C.rtloader_decref_calls)
}

func testFinalizerWhenRuntimeUnloaded(t *testing.T) {
	rtloader = newMockRtLoaderPtr()
	defer func() {
		// We have to wrap this in locks otherwise the race detector complains
		pyDestroyLock.Lock()
		rtloader = nil
		pyDestroyLock.Unlock()
	}()

	check, err := NewPythonFakeCheck(aggregator.NewNoOpSenderManager())
	if !assert.Nil(t, err) {
		return
	}

	C.reset_check_mock()
	check.instance = newMockPyObjectPtr()
	C.run_check_return = C.CString("")

	err = check.runCheck(false)
	if !assert.Nil(t, err) {
		return
	}

	// Sanity checks to ensure known start state
	assert.Equal(t, C.int(1), C.gil_locked_calls)
	assert.Equal(t, C.int(1), C.gil_unlocked_calls)
	assert.Equal(t, C.int(0), C.rtloader_incref_calls)
	assert.Equal(t, C.int(0), C.rtloader_decref_calls)
	assert.Equal(t, check.instance, C.run_check_instance)

	// Simulate rtloader being unloaded
	rtloader = nil

	pythonCheckFinalizer(check)

	// Finalizer runs in a goroutine so we have to wait a bit
	time.Sleep(100 * time.Millisecond)

	// There should be no changes in the lock/unlock and inc/decref calls
	assert.Equal(t, C.int(1), C.gil_locked_calls)
	assert.Equal(t, C.int(1), C.gil_unlocked_calls)
	assert.Equal(t, C.int(0), C.rtloader_incref_calls)
	assert.Equal(t, C.int(0), C.rtloader_decref_calls)
}

func testRunErrorNil(t *testing.T) {
	rtloader = newMockRtLoaderPtr()
	defer func() { rtloader = nil }()

	check, err := NewPythonFakeCheck(aggregator.NewNoOpSenderManager())
	if !assert.Nil(t, err) {
		return
	}

	check.instance = newMockPyObjectPtr()

	C.reset_check_mock()
	C.run_check_return = nil
	C.has_error_return = 1
	C.get_error_return = C.CString("some error")

	errStr := check.runCheck(false)
	assert.NotNil(t, errStr)
	assert.NotNil(t, fmt.Errorf("some error"), errStr)

	assert.Equal(t, C.int(1), C.gil_locked_calls)
	assert.Equal(t, C.int(1), C.gil_unlocked_calls)
	assert.Equal(t, C.int(1), C.run_check_calls)
	assert.Equal(t, C.int(0), C.get_checks_warnings_calls)

	assert.Equal(t, check.instance, C.run_check_instance)
}

func testRunErrorReturn(t *testing.T) {
	rtloader = newMockRtLoaderPtr()
	defer func() { rtloader = nil }()

	check, err := NewPythonFakeCheck(aggregator.NewNoOpSenderManager())
	if !assert.Nil(t, err) {
		return
	}

	check.instance = newMockPyObjectPtr()

	C.reset_check_mock()
	C.run_check_return = C.CString("not OK")

	errStr := check.runCheck(false)
	assert.NotNil(t, errStr)
	assert.NotNil(t, fmt.Errorf("not OK"), errStr)

	assert.Equal(t, C.int(1), C.gil_locked_calls)
	assert.Equal(t, C.int(1), C.gil_unlocked_calls)
	assert.Equal(t, C.int(1), C.run_check_calls)
	assert.Equal(t, C.int(1), C.get_checks_warnings_calls)

	assert.Equal(t, check.instance, C.run_check_instance)
}

func testRun(t *testing.T) {
	sender := mocksender.NewMockSender(checkid.ID("testID"))
	sender.SetupAcceptAll()

	rtloader = newMockRtLoaderPtr()
	defer func() { rtloader = nil }()

	c, err := NewPythonFakeCheck(sender.GetSenderManager())
	if !assert.Nil(t, err) {
		return
	}

	c.instance = newMockPyObjectPtr()
	c.id = checkid.ID("testID")

	C.reset_check_mock()
	C.run_check_return = C.CString("")

	err = c.Run()
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
	sender := mocksender.NewMockSender(checkid.ID("testID"))
	sender.SetupAcceptAll()

	rtloader = newMockRtLoaderPtr()
	defer func() { rtloader = nil }()

	c, err := NewPythonFakeCheck(sender.GetSenderManager())
	if !assert.Nil(t, err) {
		return
	}

	c.instance = newMockPyObjectPtr()
	c.id = checkid.ID("testID")

	C.reset_check_mock()
	C.run_check_return = C.CString("")

	err = c.RunSimple()
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
	rtloader = newMockRtLoaderPtr()
	defer func() { rtloader = nil }()

	senderManager := mocksender.CreateDefaultDemultiplexer()
	c, err := NewPythonFakeCheck(senderManager)
	if !assert.Nil(t, err) {
		return
	}

	c.class = newMockPyObjectPtr()

	C.reset_check_mock()

	C.get_check_return = 1
	C.get_check_check = newMockPyObjectPtr()
	err = c.Configure(senderManager, integration.FakeConfigHash, integration.Data("{\"val\": 21}"), integration.Data("{\"val\": 21}"), "test")
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
	rtloader = newMockRtLoaderPtr()
	defer func() { rtloader = nil }()

	senderManager := mocksender.CreateDefaultDemultiplexer()
	c, err := NewPythonFakeCheck(senderManager)
	if !assert.Nil(t, err) {
		return
	}

	c.class = newMockPyObjectPtr()

	C.reset_check_mock()

	C.get_check_return = 0
	C.get_check_deprecated_check = newMockPyObjectPtr()
	C.get_check_deprecated_return = 1
	err = c.Configure(senderManager, integration.FakeConfigHash, integration.Data("{\"val\": 21}"), integration.Data("{\"val\": 21}"), "test")
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

func testGetDiagnoses(t *testing.T) {
	C.reset_check_mock()

	rtloader = newMockRtLoaderPtr()
	defer func() { rtloader = nil }()

	check, err := NewPythonFakeCheck(aggregator.NewNoOpSenderManager())
	if !assert.Nil(t, err) {
		return
	}

	check.instance = newMockPyObjectPtr()

	C.get_check_diagnoses_return = C.CString(`[
		{
			"result": 0,
			"name": "foo_check_instance_a",
			"diagnosis": "All looks good",
			"category": "foo_check",
			"description": "This is description of the diagnose 1",
			"remediation": "No need to fix"
		},
		{
			"result": 1,
			"name": "foo_check_instance_b",
			"diagnosis": "All looks bad",
			"rawerror": "Strange error 2"
		}
	]`)

	diagnoses, err := check.GetDiagnoses()

	assert.Equal(t, C.int(1), C.get_check_diagnoses_calls)
	assert.Nil(t, err)
	assert.NotNil(t, diagnoses)
	assert.Equal(t, 2, len(diagnoses))

	assert.Zero(t, len(diagnoses[0].RawError))
	assert.NotZero(t, len(diagnoses[1].RawError))

	assert.Equal(t, diagnoses[0].Result, diagnosis.DiagnosisSuccess)
	assert.NotZero(t, len(diagnoses[0].Name))
	assert.NotZero(t, len(diagnoses[0].Diagnosis))
	assert.NotZero(t, len(diagnoses[0].Category))
	assert.NotZero(t, len(diagnoses[0].Description))
	assert.NotZero(t, len(diagnoses[0].Remediation))

	assert.Equal(t, diagnoses[1].Result, diagnosis.DiagnosisFail)
	assert.NotZero(t, len(diagnoses[1].Name))
	assert.NotZero(t, len(diagnoses[1].Diagnosis))
	assert.Zero(t, len(diagnoses[1].Category))
	assert.Zero(t, len(diagnoses[1].Description))
	assert.Zero(t, len(diagnoses[1].Remediation))
}

// NewPythonFakeCheck create a fake PythonCheck
func NewPythonFakeCheck(senderManager sender.SenderManager) (*PythonCheck, error) {
	c, err := NewPythonCheck(senderManager, "fake_check", nil)

	// Remove check finalizer that may trigger race condition while testing
	if err == nil {
		runtime.SetFinalizer(c, nil)
	}

	return c, err
}
