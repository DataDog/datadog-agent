// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package kubelet

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRawQuery(t *testing.T) {

	tests := []struct {
		name    string
		path    string
		body    string
		ctx     context.Context
		wantErr bool
	}{
		{
			name: "success",
			path: "/api/v1/path",
			ctx:  context.Background(),
			body: "success",
		},
		{
			name:    "failure_nil_context",
			path:    "/api/v1/path",
			ctx:     nil,
			body:    "fail",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tt.path {
					t.Errorf("Expected path: %s, got: %s", tt.path, r.URL.Path)
				}
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(tt.body))
			}))

			defer server.Close()

			kc := &kubeletClient{
				client:     http.Client{},
				kubeletURL: server.URL,
			}

			_, resp, err := kc.rawQuery(tt.ctx, tt.path)

			if tt.wantErr {
				// Check that we have en error when we want one
				require.NotNil(t, err)
			} else {
				// Validate that the response matches
				defer resp.Body.Close()
				bytes, err := io.ReadAll(resp.Body)
				if err != nil {
					t.Errorf("cannot read response bytes: %v", err)
					return
				}

				require.Equal(t, tt.body, string(bytes))

			}

		})
	}

}
