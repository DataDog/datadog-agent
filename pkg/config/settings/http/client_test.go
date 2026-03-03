// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package http implements helpers for the runtime settings HTTP API
package http

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	"github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
)

func TestNewSecureClientOptions(t *testing.T) {
	ipcComp := ipcmock.New(t)

	var settingsHandler http.HandlerFunc = func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(time.Second * 1)
		w.WriteHeader(http.StatusOK)
	}
	srv := ipcComp.NewMockServer(settingsHandler)

	type args struct {
		c                 ipc.HTTPClient
		baseURL           string
		targetProcessName string
		clientOptions     []ipc.RequestOption
	}
	tests := []struct {
		name          string
		args          args
		shouldTimeout bool
	}{
		{
			name: "without option [should work]",
			args: args{
				c:                 ipcComp.GetClient(),
				baseURL:           srv.URL,
				targetProcessName: "test-process",
			},
			shouldTimeout: false,
		},
		{
			name: "with 500ms timeout [should fail]",
			args: args{
				c:                 ipcComp.GetClient(),
				baseURL:           srv.URL,
				targetProcessName: "test-process",
				clientOptions:     []ipc.RequestOption{httphelpers.WithTimeout(500 * time.Millisecond)},
			},
			shouldTimeout: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewSecureClient(tt.args.c, tt.args.baseURL, tt.args.targetProcessName, tt.args.clientOptions...)

			_, err := client.FullConfig()
			if tt.shouldTimeout {
				// not able to use require.ErrorIs since the wrapper error is private (net/http.timeoutError)
				require.ErrorContains(t, err, "context deadline exceeded (Client.Timeout exceeded while awaiting headers)")
			} else {
				require.NoError(t, err)
			}
		})
	}
}
