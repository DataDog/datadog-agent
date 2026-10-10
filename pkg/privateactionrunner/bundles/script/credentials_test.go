// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_script

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
)

func TestParseCredentials(t *testing.T) {
	tests := []struct {
		name     string
		contents string
		wantErr  bool
	}{
		{
			name: "valid configuration",
			contents: `schemaId: script-credentials-v1
runPredefinedScript:
  hello:
    command: [echo, hello]
`,
		},
		{name: "invalid YAML", contents: "sensitive-scalar: [", wantErr: true},
		{name: "invalid schema ID", contents: "schemaId: sensitive-schema-id", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := parseCredentials(scriptCredentials(tt.contents))
			if tt.wantErr {
				require.Error(t, err)
				assert.Equal(t, "invalid script configuration", err.Error())
				assert.NotContains(t, err.Error(), "sensitive-scalar")
				assert.NotContains(t, err.Error(), "sensitive-schema-id")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, []string{"echo", "hello"}, config.RunPredefinedScript["hello"].Command)
		})
	}
}

func scriptCredentials(contents string) *privateconnection.PrivateCredentials {
	return &privateconnection.PrivateCredentials{
		Type: privateconnection.TokenAuthType,
		Tokens: []privateconnection.PrivateCredentialsToken{
			{Name: "configFileLocation", Value: contents},
		},
	}
}
