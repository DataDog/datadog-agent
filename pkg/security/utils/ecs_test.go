// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetECSTaskID(t *testing.T) {
	const containerPath = "/v4/bfa2636268144d039771334145e490c5-1117626119"

	tests := []struct {
		name    string
		status  int
		body    string
		taskID  string
		wantErr bool
	}{
		{
			name:   "task",
			status: http.StatusOK,
			body:   `{"Cluster":"MyCluster","TaskARN":"arn:aws:ecs:us-east-1:123456789012:task/MyCluster/BFA2636268144D039771334145E490C5"}`,
			taskID: "bfa2636268144d039771334145e490c5",
		},
		{
			name:    "error-status",
			status:  http.StatusInternalServerError,
			body:    `{}`,
			wantErr: true,
		},
		{
			name:    "invalid-json",
			status:  http.StatusOK,
			body:    `not json`,
			wantErr: true,
		},
		{
			name:    "missing-task-arn",
			status:  http.StatusOK,
			body:    `{"Cluster":"MyCluster"}`,
			wantErr: true,
		},
		{
			name:    "invalid-task-arn",
			status:  http.StatusOK,
			body:    `{"TaskARN":"arn:aws:ecs:us-east-1:123456789012:task-definition/web:1"}`,
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != containerPath+"/task" {
					http.NotFound(w, r)
					return
				}
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()

			taskID, err := GetECSTaskID(t.Context(), server.URL+containerPath)
			if test.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.taskID, taskID)
		})
	}
}
