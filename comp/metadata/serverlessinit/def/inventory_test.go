// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serverlessinit

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPayloadValid(t *testing.T) {
	assert.True(t, PayloadValid(map[string]interface{}{
		"resource_id": "/subscriptions/sub/resourcegroups/rg/providers/microsoft.web/sites/site",
	}))

	// Unrelated optional fields do not affect validity.
	assert.True(t, PayloadValid(map[string]interface{}{
		"resource_id": "test-resource",
		"runtime":     nil,
		"region":      nil,
	}))

	for name, resourceID := range map[string]interface{}{
		"absent":      nil,
		"nil cleared": nil,
		"empty":       "",
	} {
		t.Run(name, func(t *testing.T) {
			assert.False(t, PayloadValid(map[string]interface{}{"resource_id": resourceID}))
		})
	}
}
