// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python && test

package python

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
#include <datadog_agent_rtloader.h>
#include <stdlib.h>
#include <string.h>

extern int get_class_dd_wheel_return;
extern rtloader_pyobject_t *get_class_dd_wheel_py_module;
extern rtloader_pyobject_t *get_class_dd_wheel_py_class;
extern int get_class_return;
extern rtloader_pyobject_t *get_class_py_module;
extern rtloader_pyobject_t *get_class_py_class;
extern void reset_loader_mock();

extern int has_error_return;
extern char *get_error_return;

int discover_config_calls = 0;
rtloader_pyobject_t *discover_config_py_class = NULL;
const char *discover_config_service_json = NULL;
char *discover_config_return = NULL;

char *discover_config(rtloader_t *rtloader, rtloader_pyobject_t *py_class, const char *service_json) {
	discover_config_calls++;
	discover_config_py_class = py_class;
	if (discover_config_service_json != NULL) {
		free((void *)discover_config_service_json);
	}
	discover_config_service_json = strdup(service_json);

	return discover_config_return;
}

void reset_discovery_mock() {
	discover_config_calls = 0;
	discover_config_py_class = NULL;
	if (discover_config_service_json != NULL) {
		free((void *)discover_config_service_json);
	}
	discover_config_service_json = NULL;
	if (discover_config_return != NULL) {
		free(discover_config_return);
	}
	discover_config_return = NULL;
}
*/
import "C"

func setupDiscoveryTest(t *testing.T) {
	mockRtloader(t)
	C.reset_loader_mock()
	C.reset_discovery_mock()
	C.has_error_return = 0
	C.get_error_return = C.CString("")
	t.Cleanup(func() {
		C.reset_discovery_mock()
	})

	C.get_class_dd_wheel_return = 1
	C.get_class_dd_wheel_py_module = newMockPyObjectPtr()
	C.get_class_dd_wheel_py_class = newMockPyObjectPtr()
}

func testDiscoverConfig(t *testing.T) {
	setupDiscoveryTest(t)
	resultJSON := `[{"init_config":{"enabled":true},"instances":[{"url":"http://10.0.0.1:8080"}],"logs":[{"source":"fake"}]}]`
	serviceJSON := `{"id":"svc","host":"10.0.0.1","ports":[{"number":8080,"name":"http"}]}`
	C.discover_config_return = C.CString(resultJSON)

	got, err := DiscoverConfig("fake_check", serviceJSON)

	require.NoError(t, err)
	assert.Equal(t, resultJSON, got)
	assert.Equal(t, 1, int(C.discover_config_calls))
	assert.Equal(t, C.get_class_dd_wheel_py_class, C.discover_config_py_class)
	assert.Equal(t, serviceJSON, C.GoString(C.discover_config_service_json))
}

func testDiscoverConfigCustomCheck(t *testing.T) {
	setupDiscoveryTest(t)
	C.get_class_dd_wheel_return = 0
	C.get_class_dd_wheel_py_module = nil
	C.get_class_dd_wheel_py_class = nil
	C.get_class_return = 1
	C.get_class_py_module = newMockPyObjectPtr()
	C.get_class_py_class = newMockPyObjectPtr()
	resultJSON := `[{"instances":[{"url":"http://10.0.0.1:8080"}]}]`
	C.discover_config_return = C.CString(resultJSON)

	got, err := DiscoverConfig("fake_check", `{"id":"svc","host":"10.0.0.1","ports":[]}`)

	require.NoError(t, err)
	assert.Equal(t, resultJSON, got)
	assert.Equal(t, C.get_class_py_class, C.discover_config_py_class)
}

func testDiscoverConfigNoConfigs(t *testing.T) {
	for _, result := range []string{"null", "[]"} {
		t.Run(result, func(t *testing.T) {
			setupDiscoveryTest(t)
			C.discover_config_return = C.CString(result)

			got, err := DiscoverConfig("fake_check", `{"id":"svc","host":"10.0.0.1","ports":[]}`)

			require.NoError(t, err)
			assert.Equal(t, result, got)
		})
	}
}

func testDiscoverConfigRtloaderError(t *testing.T) {
	setupDiscoveryTest(t)
	C.discover_config_return = nil
	C.has_error_return = 1
	C.get_error_return = C.CString("discover failed")

	got, err := DiscoverConfig("fake_check", `{"id":"svc","host":"10.0.0.1","ports":[]}`)

	require.Error(t, err)
	assert.Empty(t, got)
	assert.Contains(t, err.Error(), "discover failed")
}

func testDiscoverConfigReturnsMalformedResult(t *testing.T) {
	setupDiscoveryTest(t)
	C.discover_config_return = C.CString("{")

	got, err := DiscoverConfig("fake_check", `{"id":"svc","host":"10.0.0.1","ports":[]}`)

	require.NoError(t, err)
	assert.Equal(t, "{", got)
}
