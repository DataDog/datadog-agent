// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python && test

package python

import (
	"runtime"
	"strings"
	"testing"
	"time"

	workloadfilterfxmock "github.com/DataDog/datadog-agent/comp/core/workloadfilter/fx-mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	nooptagger "github.com/DataDog/datadog-agent/comp/core/tagger/impl-noop"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/option"
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

int get_attr_bool_return = 0;
rtloader_pyobject_t *get_attr_bool_py_class = NULL;
const char *get_attr_bool_attr_name = NULL;
int get_attr_bool_attr_value = 0;

int get_attr_bool(rtloader_t *rtloader, rtloader_pyobject_t *py_class, const char *attr_name, bool *value) {
	get_attr_bool_py_class = py_class;
	get_attr_bool_attr_name = attr_name;
	*value = get_attr_bool_attr_value;

	return get_attr_bool_return;
}

extern int get_check_return;
extern int get_check_calls;
extern rtloader_pyobject_t *get_check_py_class;
extern const char *get_check_init_config;
extern const char *get_check_instance;
extern const char *get_check_check_id;
extern const char *get_check_check_name;
extern rtloader_pyobject_t *get_check_check;

int get_check(rtloader_t *rtloader, rtloader_pyobject_t *py_class, const char *init_config, const char *instance,
const char *check_id, const char *check_name, rtloader_pyobject_t **check);

// get_check_deprecated MOCK

extern int get_check_deprecated_calls;
extern int get_check_deprecated_return;
extern rtloader_pyobject_t *get_check_deprecated_py_class;
extern const char *get_check_deprecated_init_config;
extern const char *get_check_deprecated_instance;
extern const char *get_check_deprecated_check_id;
extern const char *get_check_deprecated_check_name;
extern const char *get_check_deprecated_agent_config;
extern rtloader_pyobject_t *get_check_deprecated_check;

int get_check_deprecated(rtloader_t *rtloader, rtloader_pyobject_t *py_class, const char *init_config,
const char *instance, const char *agent_config, const char *check_id, const char *check_name,
rtloader_pyobject_t **check);

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

	get_attr_bool_return = 0;
	get_attr_bool_py_class = NULL;
	get_attr_bool_attr_name = NULL;
	get_attr_bool_attr_value = 0;

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

func testLoadCustomCheck(t *testing.T) {
	C.reset_loader_mock()

	conf := integration.Config{
		Name:       "fake_check",
		Instances:  []integration.Data{integration.Data("{\"value\": 1}")},
		InitConfig: integration.Data("{}"),
		Source:     "fake_check:/etc/datadog-agent/conf.d/fake_check.yaml",
	}

	mockRtloader(t)

	senderManager := mocksender.CreateDefaultDemultiplexer()
	logReceiver := option.None[integrations.Component]()
	tagger := nooptagger.NewComponent()
	loader, err := NewPythonCheckLoader(senderManager, logReceiver, tagger, nil)
	assert.Nil(t, err)

	// testing loading custom checks
	C.get_class_return = 1
	C.get_class_py_module = newMockPyObjectPtr()
	C.get_class_py_class = newMockPyObjectPtr()
	C.get_attr_string_return = 0
	C.get_check_return = 0
	C.get_check_deprecated_check = newMockPyObjectPtr()
	C.get_check_deprecated_return = 1

	check, err := loader.Load(senderManager, conf, conf.Instances[0], 1)
	// Remove check finalizer that may trigger race condition while testing
	runtime.SetFinalizer(check, nil)

	assert.Nil(t, err)
	assert.Equal(t, "fake_check", check.(*PythonCheck).ModuleName)
	assert.Equal(t, "unversioned", check.(*PythonCheck).version)
	assert.Equal(t, C.get_class_py_class, check.(*PythonCheck).class)
	assert.Equal(t, "fake_check:/etc/datadog-agent/conf.d/fake_check.yaml[1]", check.(*PythonCheck).source)
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

	mockRtloader(t)

	senderManager := mocksender.CreateDefaultDemultiplexer()
	logReceiver := option.None[integrations.Component]()
	tagger := nooptagger.NewComponent()
	filterStore := workloadfilterfxmock.SetupMockFilter(t)
	loader, err := NewPythonCheckLoader(senderManager, logReceiver, tagger, filterStore)
	assert.Nil(t, err)

	// testing loading dd wheels
	C.get_class_dd_wheel_return = 1
	C.get_class_dd_wheel_py_module = newMockPyObjectPtr()
	C.get_class_dd_wheel_py_class = newMockPyObjectPtr()
	C.get_attr_string_return = 1
	C.get_attr_string_attr_value = C.CString("1.2.3")
	C.get_check_return = 0
	C.get_check_deprecated_check = newMockPyObjectPtr()
	C.get_check_deprecated_return = 1

	check, err := loader.Load(senderManager, conf, conf.Instances[0], 0)
	// Remove check finalizer that may trigger race condition while testing
	runtime.SetFinalizer(check, nil)

	assert.Nil(t, err)
	assert.Equal(t, "fake_check", check.(*PythonCheck).ModuleName)
	assert.Equal(t, "1.2.3", check.(*PythonCheck).version)
	assert.Equal(t, C.get_class_dd_wheel_py_class, check.(*PythonCheck).class)
	// test we call get_attr_string on the module
	assert.Equal(t, C.get_attr_string_py_class, C.get_class_dd_wheel_py_module)
}

func testLoadHACheck(t *testing.T) {
	conf := integration.Config{
		Name:       "fake_check",
		Instances:  []integration.Data{integration.Data("{\"value\": 1}")},
		InitConfig: integration.Data("{}"),
	}

	mockRtloader(t)

	senderManager := mocksender.CreateDefaultDemultiplexer()
	logReceiver := option.None[integrations.Component]()
	tagger := nooptagger.NewComponent()
	loader, err := NewPythonCheckLoader(senderManager, logReceiver, tagger, nil)
	assert.Nil(t, err)

	testCases := []struct {
		name                string
		haAgentEnabled      bool
		getAttrBoolReturn   int
		getAttrBoolValue    int
		expectedHaSupported bool
		expectedGetAttrRun  bool
	}{
		{
			name:                "HA Agent not enabled",
			haAgentEnabled:      false,
			getAttrBoolReturn:   0,
			getAttrBoolValue:    0,
			expectedHaSupported: false,
			expectedGetAttrRun:  false,
		}, {
			name:                "get_attr_bool returns an error",
			haAgentEnabled:      true,
			getAttrBoolReturn:   0,
			getAttrBoolValue:    0,
			expectedHaSupported: false,
			expectedGetAttrRun:  true,
		}, {
			name:                "get_attr_bool returns false",
			haAgentEnabled:      true,
			getAttrBoolReturn:   1,
			getAttrBoolValue:    0,
			expectedHaSupported: false,
			expectedGetAttrRun:  true,
		}, {
			name:                "get_attr_bool returns true",
			haAgentEnabled:      true,
			getAttrBoolReturn:   1,
			getAttrBoolValue:    1,
			expectedHaSupported: true,
			expectedGetAttrRun:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			C.reset_loader_mock()

			pkgconfigsetup.Datadog().SetInTest("ha_agent.enabled", tc.haAgentEnabled)

			// testing loading custom checks
			C.get_class_return = 1
			C.get_class_py_module = newMockPyObjectPtr()
			C.get_class_py_class = newMockPyObjectPtr()
			C.get_attr_string_return = 0
			C.get_check_return = 0
			C.get_check_deprecated_check = newMockPyObjectPtr()
			C.get_check_deprecated_return = 1

			C.get_attr_bool_return = C.int(tc.getAttrBoolReturn)
			C.get_attr_bool_attr_value = C.int(tc.getAttrBoolValue)

			check, err := loader.Load(senderManager, conf, conf.Instances[0], 0)
			// Remove check finalizer that may trigger race condition while testing
			runtime.SetFinalizer(check, nil)

			assert.Nil(t, err)
			assert.Equal(t, "fake_check", check.(*PythonCheck).ModuleName)
			assert.Equal(t, "unversioned", check.(*PythonCheck).version)
			assert.Equal(t, C.get_class_py_class, check.(*PythonCheck).class)
			// test we call get_attr_bool on the class if get_attr_bool is run
			assert.Equal(t, tc.expectedGetAttrRun, C.get_attr_bool_py_class == C.get_class_py_class)
			assert.Equal(t, tc.expectedHaSupported, check.(*PythonCheck).haSupported)
		})
	}
}

func testLoadError(t *testing.T) {
	C.reset_loader_mock()

	conf := integration.Config{
		Name:       "fake_check",
		Instances:  []integration.Data{integration.Data("{\"value\": 1}")},
		InitConfig: integration.Data("{}"),
	}

	mockRtloader(t)

	senderManager := mocksender.CreateDefaultDemultiplexer()
	logReceiver := option.None[integrations.Component]()
	tagger := nooptagger.NewComponent()
	loader, err := NewPythonCheckLoader(senderManager, logReceiver, tagger, nil)
	require.NoError(t, err)

	// testing loading dd wheels
	C.get_class_dd_wheel_return = 0
	C.get_class_return = 0
	C.get_class_dd_wheel_py_module = nil
	C.get_class_dd_wheel_py_class = nil

	check, err := loader.Load(senderManager, conf, conf.Instances[0], 0)
	require.Error(t, err)
	require.Nil(t, check)
}

// testLoadCustomCheckEmitsCheckReadyMetric verifies that loading a custom check
// emits the datadog.agent.check_ready metric with the correct check_name tag.
// This test ensures that the regression from version 7.73 (where check_name was empty)
// doesn't happen again.
func testLoadCustomCheckEmitsCheckReadyMetric(t *testing.T) {
	C.reset_loader_mock()

	// Clear the py3Linted cache and recurrent series to ensure clean state
	py3LintedLock.Lock()
	delete(py3Linted, "check_ready_test_check")
	py3LintedLock.Unlock()
	aggregator.ClearRecurrentSeries()

	conf := integration.Config{
		Name:       "check_ready_test_check",
		Instances:  []integration.Data{integration.Data("{\"value\": 1}")},
		InitConfig: integration.Data("{}"),
	}

	mockRtloader(t)

	// Ensure py3 validation is enabled (default)
	pkgconfigsetup.Datadog().SetInTest("disable_py3_validation", false)

	senderManager := mocksender.CreateDefaultDemultiplexer()
	logReceiver := option.None[integrations.Component]()
	tagger := nooptagger.NewComponent()
	loader, err := NewPythonCheckLoader(senderManager, logReceiver, tagger, nil)
	require.NoError(t, err)

	// Setup for loading a custom check (not a wheel)
	C.get_class_return = 1
	C.get_class_py_module = newMockPyObjectPtr()
	C.get_class_py_class = newMockPyObjectPtr()
	C.get_attr_string_return = 0
	C.get_check_return = 0
	C.get_check_deprecated_check = newMockPyObjectPtr()
	C.get_check_deprecated_return = 1

	check, err := loader.Load(senderManager, conf, conf.Instances[0], 0)
	runtime.SetFinalizer(check, nil)
	require.NoError(t, err)

	// Wait for the reportPy3Warnings goroutine to complete
	time.Sleep(100 * time.Millisecond)

	// Verify that the check_ready metric was emitted with the correct check_name tag
	series := aggregator.GetRecurrentSeries()
	var foundCheckReadyMetric bool
	var checkNameTag string

	for _, serie := range series {
		if serie.Name == "datadog.agent.check_ready" {
			foundCheckReadyMetric = true
			serie.Tags.ForEach(func(tag string) {
				if strings.HasPrefix(tag, "check_name:") {
					checkNameTag = tag
				}
			})
			break
		}
	}

	assert.True(t, foundCheckReadyMetric, "datadog.agent.check_ready metric should be emitted")
	assert.Equal(t, "check_name:check_ready_test_check", checkNameTag,
		"check_name tag should contain the check name, not be empty")
}
