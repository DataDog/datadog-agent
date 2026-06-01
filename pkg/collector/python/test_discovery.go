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

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
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
	C.discover_config_return = C.CString(`[{"url":"http://10.0.0.1:8080"}]`)

	configs, err := DiscoverConfig("fake_check", DiscoveryService{
		ID:   "svc",
		Host: "10.0.0.1",
		Ports: []DiscoveryPort{
			{Number: 8080, Name: "http"},
		},
	})

	require.NoError(t, err)
	assert.Equal(t, []integration.Data{integration.Data(`{"url":"http://10.0.0.1:8080"}`)}, configs)
	assert.Equal(t, 1, int(C.discover_config_calls))
	assert.Equal(t, C.get_class_dd_wheel_py_class, C.discover_config_py_class)
	assert.Equal(t, `{"id":"svc","host":"10.0.0.1","ports":[{"number":8080,"name":"http"}]}`, C.GoString(C.discover_config_service_json))
}

func testDiscoverConfigCustomCheck(t *testing.T) {
	setupDiscoveryTest(t)
	C.get_class_dd_wheel_return = 0
	C.get_class_dd_wheel_py_module = nil
	C.get_class_dd_wheel_py_class = nil
	C.get_class_return = 1
	C.get_class_py_module = newMockPyObjectPtr()
	C.get_class_py_class = newMockPyObjectPtr()
	C.discover_config_return = C.CString(`[{"url":"http://10.0.0.1:8080"}]`)

	configs, err := DiscoverConfig("fake_check", DiscoveryService{
		ID:    "svc",
		Host:  "10.0.0.1",
		Ports: []DiscoveryPort{},
	})

	require.NoError(t, err)
	assert.Equal(t, []integration.Data{integration.Data(`{"url":"http://10.0.0.1:8080"}`)}, configs)
	assert.Equal(t, C.get_class_py_class, C.discover_config_py_class)
}

func testDiscoverConfigNoConfigs(t *testing.T) {
	for _, result := range []string{"null", "[]"} {
		t.Run(result, func(t *testing.T) {
			setupDiscoveryTest(t)
			C.discover_config_return = C.CString(result)

			configs, err := DiscoverConfig("fake_check", DiscoveryService{
				ID:    "svc",
				Host:  "10.0.0.1",
				Ports: []DiscoveryPort{},
			})

			require.NoError(t, err)
			assert.Empty(t, configs)
		})
	}
}

func testDiscoverConfigRtloaderError(t *testing.T) {
	setupDiscoveryTest(t)
	C.discover_config_return = nil
	C.has_error_return = 1
	C.get_error_return = C.CString("discover failed")

	configs, err := DiscoverConfig("fake_check", DiscoveryService{
		ID:    "svc",
		Host:  "10.0.0.1",
		Ports: []DiscoveryPort{},
	})

	require.Error(t, err)
	assert.Nil(t, configs)
	assert.Contains(t, err.Error(), "discover failed")
}

func testDiscoverConfigMalformedResult(t *testing.T) {
	setupDiscoveryTest(t)
	C.discover_config_return = C.CString("{")

	configs, err := DiscoverConfig("fake_check", DiscoveryService{
		ID:    "svc",
		Host:  "10.0.0.1",
		Ports: []DiscoveryPort{},
	})

	require.Error(t, err)
	assert.Nil(t, configs)
}
