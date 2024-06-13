// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/agent/common/signals"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
)

func TestGetHostname(t *testing.T) {
	req, err := http.NewRequest("GET", "/hostname", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(getHostname)
	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)

	var response string
	err = json.Unmarshal(rr.Body.Bytes(), &response)
	require.NoError(t, err)

	expectedHostname, _ := hostname.Get(context.Background())
	expectedResponse, _ := json.Marshal(expectedHostname)

	require.Equal(t, expectedResponse, rr.Body.Bytes())
}

func TestStopAgent(t *testing.T) {
	// Make a channel to exit the function
	stopCh := make(chan error)

	go func() {
		select {
		case <-signals.Stopper:
			t.Log("Received stop command, shutting down...")
			stopCh <- nil
		case <-time.After(time.Second * 30): // Timeout after 5 seconds
			stopCh <- fmt.Errorf("Timeout waiting for stop signal")
		}
	}()

	req, err := http.NewRequest("POST", "/stop", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(stopAgent)
	handler.ServeHTTP(rr, req)

	require.NoError(t, <-stopCh)

	require.Equal(t, http.StatusOK, rr.Code)
}
