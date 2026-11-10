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
	const secret = "sensitive-marker"
	tests := []struct {
		name     string
		contents string
		wantErr  string
	}{
		{
			name: "valid configuration",
			contents: `schemaId: script-credentials-v1
runPredefinedScript:
  hello:
    command: [echo, hello]
`,
		},
		{name: "plain scalar such as a token", contents: secret, wantErr: "invalid script configuration: not valid YAML"},
		{name: "syntax error", contents: secret + ": [", wantErr: "invalid script configuration: not valid YAML"},
		{name: "wrong schemaId", contents: "schemaId: " + secret, wantErr: `invalid script configuration: schemaId must be "script-credentials-v1"`},
		{name: "missing schemaId", contents: "runPredefinedScript: {}", wantErr: `invalid script configuration: schemaId must be "script-credentials-v1"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			credentials := &privateconnection.PrivateCredentials{
				Type:   privateconnection.TokenAuthType,
				Tokens: []privateconnection.PrivateCredentialsToken{{Name: "configFileLocation", Value: tt.contents}},
			}

			config, err := parseCredentials(credentials)

			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, []string{"echo", "hello"}, config.RunPredefinedScript["hello"].Command)
		})
	}
}
