// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serverlessinit

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInsufficientPayload(t *testing.T) {
	assert.NoError(t, InsufficientPayload(map[string]interface{}{
		"resource_id": "/subscriptions/sub/resourcegroups/rg/providers/microsoft.web/sites/site",
	}))

	// Unrelated optional fields do not affect sufficiency.
	assert.NoError(t, InsufficientPayload(map[string]interface{}{
		"resource_id": "test-resource",
		"runtime":     nil,
		"region":      nil,
	}))

	for _, tt := range []struct {
		name   string
		fields map[string]interface{}
		want   string
	}{
		{name: "absent", fields: map[string]interface{}{}, want: `missing required field "resource_id"`},
		{name: "nil clears", fields: map[string]interface{}{"resource_id": nil}, want: `missing required field "resource_id"`},
		{name: "empty string", fields: map[string]interface{}{"resource_id": ""}, want: `required field "resource_id" is empty`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := InsufficientPayload(tt.fields)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}
