// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package actions

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSplitFQN(t *testing.T) {
	tests := []struct {
		name           string
		fqn            string
		expectedBundle string
		expectedAction string
	}{
		{
			name:           "standard FQN",
			fqn:            "com.datadoghq.script.runScript",
			expectedBundle: "com.datadoghq.script",
			expectedAction: "runScript",
		},
		{
			name:           "short FQN",
			fqn:            "bundle.action",
			expectedBundle: "bundle",
			expectedAction: "action",
		},
		{
			name:           "single segment - no dot",
			fqn:            "action",
			expectedBundle: "",
			expectedAction: "",
		},
		{
			name:           "empty string",
			fqn:            "",
			expectedBundle: "",
			expectedAction: "",
		},
		{
			name:           "multiple dots in bundle",
			fqn:            "com.datadoghq.kubernetes.core.getResource",
			expectedBundle: "com.datadoghq.kubernetes.core",
			expectedAction: "getResource",
		},
		{
			name:           "trailing dot",
			fqn:            "bundle.",
			expectedBundle: "bundle",
			expectedAction: "",
		},
		{
			name:           "leading dot",
			fqn:            ".action",
			expectedBundle: "",
			expectedAction: "action",
		},
		{
			name:           "only dot",
			fqn:            ".",
			expectedBundle: "",
			expectedAction: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bundle, action := SplitFQN(tt.fqn)
			assert.Equal(t, tt.expectedBundle, bundle)
			assert.Equal(t, tt.expectedAction, action)
		})
	}
}
