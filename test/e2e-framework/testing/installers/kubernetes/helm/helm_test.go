// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package helm

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
