// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package metricresolution_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/metricresolution"
)

func TestEnabled(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		expected bool
	}{
		{name: "enabled", value: "true", expected: true},
		{name: "disabled", value: "false", expected: false},
		{name: "invalid", value: "not-a-bool", expected: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(metricresolution.EnabledEnvVar, test.value)
			require.Equal(t, test.expected, metricresolution.Enabled())
		})
	}
}
