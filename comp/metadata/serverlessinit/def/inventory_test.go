// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serverlessinit

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidatePayload(t *testing.T) {
	assert.NoError(t, ValidatePayload(map[string]interface{}{
		"resource_id": "/subscriptions/sub/resourcegroups/rg/providers/microsoft.web/sites/site",
	}))

	// Unrelated optional fields do not affect validity.
	assert.NoError(t, ValidatePayload(map[string]interface{}{
		"resource_id": "test-resource",
		"runtime":     nil,
		"region":      nil,
	}))

	for name, resourceID := range map[string]interface{}{
		"absent":       nil,
		"nil cleared":  nil,
		"empty":        "",
		"blank":        "  ",
		"not a string": 123,
	} {
		t.Run(name, func(t *testing.T) {
			err := ValidatePayload(map[string]interface{}{"resource_id": resourceID})
			assert.EqualError(t, err, "resource id is missing or empty")
		})
	}
}
