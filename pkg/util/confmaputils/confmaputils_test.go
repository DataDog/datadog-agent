// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package confmaputils

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/confmap/xconfmap"
)

func TestGetExpandedValue(t *testing.T) {
	cm := ConfMap{
		"string_value": xconfmap.ExpandedValue{Value: "expanded-string", Original: "${ENV_VAR}"},
		"bool_value":   xconfmap.ExpandedValue{Value: true, Original: "${BOOL_VAR}"},
		"int_value":    xconfmap.ExpandedValue{Value: 42, Original: "${INT_VAR}"},
	}

	strVal, ok := Get[string](cm, "string_value")
	require.True(t, ok)
	require.Equal(t, "expanded-string", strVal)

	boolVal, ok := Get[bool](cm, "bool_value")
	require.True(t, ok)
	require.Equal(t, true, boolVal)

	intVal, ok := Get[int](cm, "int_value")
	require.True(t, ok)
	require.Equal(t, 42, intVal)

	// Getting the ExpandedValue itself should still work
	expVal, ok := Get[xconfmap.ExpandedValue](cm, "string_value")
	require.True(t, ok)
	require.Equal(t, "expanded-string", expVal.Value)
	require.Equal(t, "${ENV_VAR}", expVal.Original)
}
