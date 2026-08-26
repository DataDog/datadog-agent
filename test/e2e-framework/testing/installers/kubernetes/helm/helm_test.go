// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package helm

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
)

func TestMergeMaps(t *testing.T) {
	dst := map[string]interface{}{
		"agents": map[string]interface{}{
			"useHostNetwork": true,
			"image":          map[string]interface{}{"tag": "latest"},
		},
		"datadog": map[string]interface{}{"apiKey": "abc"},
	}
	src := map[string]interface{}{
		"agents": map[string]interface{}{
			"useHostNetwork": false,
		},
		"newTopLevel": "value",
	}

	mergeMaps(dst, src)

	assert.Equal(t, false, dst["agents"].(map[string]interface{})["useHostNetwork"])
	assert.Equal(t, "latest", dst["agents"].(map[string]interface{})["image"].(map[string]interface{})["tag"])
	assert.Equal(t, "abc", dst["datadog"].(map[string]interface{})["apiKey"])
	assert.Equal(t, "value", dst["newTopLevel"])
}

func TestMergeMapsOverridesMapWithNonMap(t *testing.T) {
	dst := map[string]interface{}{"agents": map[string]interface{}{"image": map[string]interface{}{"tag": "latest"}}}
	src := map[string]interface{}{"agents": "disabled"}

	mergeMaps(dst, src)

	assert.Equal(t, "disabled", dst["agents"])
}

func TestBuildValuesConfiguresFakeintakeRemoteConfig(t *testing.T) {
	values := buildValues(
		chartParams{RCRootJSON: "root-json"},
		"kind-cluster",
		&fakeintake.FakeintakeOutput{URL: "http://fakeintake"},
		"cluster-agent-token",
	)

	expected := []interface{}{
		map[string]interface{}{"name": "DD_REMOTE_CONFIGURATION_RC_DD_URL", "value": "http://fakeintake"},
		map[string]interface{}{"name": "DD_REMOTE_CONFIGURATION_NO_TLS", "value": "true"},
		map[string]interface{}{"name": "DD_REMOTE_CONFIGURATION_NO_TLS_VALIDATION", "value": "true"},
		map[string]interface{}{"name": "DD_REMOTE_CONFIGURATION_CONFIG_ROOT", "value": "root-json"},
		map[string]interface{}{"name": "DD_REMOTE_CONFIGURATION_DIRECTOR_ROOT", "value": "root-json"},
		map[string]interface{}{"name": "DD_REMOTE_CONFIGURATION_REFRESH_INTERVAL", "value": "5s"},
	}

	assert.Equal(t, expected, values["datadog"].(map[string]interface{})["env"])
	assert.Equal(t, expected, values["clusterAgent"].(map[string]interface{})["env"])
}
