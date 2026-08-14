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
	// Original holds the substituted text; Value is the YAML-parsed form of that text.
	// For a plain string, both are equivalent string representations.
	cm := ConfMap{
		"string_value": xconfmap.ExpandedValue{Value: "expanded-string", Original: "expanded-string"},
		"bool_value":   xconfmap.ExpandedValue{Value: true, Original: "true"},
		"int_value":    xconfmap.ExpandedValue{Value: 42, Original: "42"},
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
	require.Equal(t, "expanded-string", expVal.Original)
}

func TestGetExpandedValueStringFromNonStringScalar(t *testing.T) {
	// When an env var value looks like a YAML scalar (int, bool), OTel parses Value
	// as that type but keeps the raw substituted text in Original. Get[string] must
	// return Original so callers see a string rather than ok=false.
	cm := ConfMap{
		"api_key":    xconfmap.ExpandedValue{Value: 12345, Original: "12345"},
		"feature_on": xconfmap.ExpandedValue{Value: true, Original: "true"},
	}

	apiKey, ok := Get[string](cm, "api_key")
	require.True(t, ok)
	require.Equal(t, "12345", apiKey)

	feature, ok := Get[string](cm, "feature_on")
	require.True(t, ok)
	require.Equal(t, "true", feature)

	// Non-string requests still use Value
	boolVal, ok := Get[bool](cm, "feature_on")
	require.True(t, ok)
	require.Equal(t, true, boolVal)
}
