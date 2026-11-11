// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package nodename

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFromEnv(t *testing.T) {
	tests := []struct {
		name    string
		envVars string
		env     map[string]string
		// wantNodeName and wantEnvVar are also checked when wantErr is set.
		wantNodeName string
		wantEnvVar   string
		wantErr      string
	}{
		{
			name:         "first set variable wins",
			envVars:      "FIRST_NODE_VAR,SECOND_NODE_VAR",
			env:          map[string]string{"FIRST_NODE_VAR": "first-node", "SECOND_NODE_VAR": "second-node"},
			wantNodeName: "first-node", wantEnvVar: "FIRST_NODE_VAR",
		},
		{
			name:         "unset and blank variables are skipped",
			envVars:      "FIRST_NODE_VAR,SECOND_NODE_VAR,THIRD_NODE_VAR",
			env:          map[string]string{"SECOND_NODE_VAR": " \n", "THIRD_NODE_VAR": "third-node"},
			wantNodeName: "third-node", wantEnvVar: "THIRD_NODE_VAR",
		},
		{
			name:         "whitespace around names and value is trimmed",
			envVars:      " FIRST_NODE_VAR , SECOND_NODE_VAR ",
			env:          map[string]string{"SECOND_NODE_VAR": " second-node\n"},
			wantNodeName: "second-node", wantEnvVar: "SECOND_NODE_VAR",
		},
		{
			name:    "none set",
			envVars: "FIRST_NODE_VAR,SECOND_NODE_VAR",
			wantErr: `none of the environment variables in "FIRST_NODE_VAR,SECOND_NODE_VAR" is set`,
		},
		{
			name:         "invalid node name",
			envVars:      "FIRST_NODE_VAR",
			env:          map[string]string{"FIRST_NODE_VAR": "Not a node name"},
			wantNodeName: "Not a node name", wantEnvVar: "FIRST_NODE_VAR",
			wantErr: `environment variable "FIRST_NODE_VAR" holds "Not a node name", which isn't a valid node name`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, name := range []string{"FIRST_NODE_VAR", "SECOND_NODE_VAR", "THIRD_NODE_VAR"} {
				t.Setenv(name, tt.env[name])
			}

			nodeName, envVar, err := FromEnv(tt.envVars)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantNodeName, nodeName)
			assert.Equal(t, tt.wantEnvVar, envVar)
		})
	}
}
