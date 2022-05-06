// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"text/template"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/process-agent/api"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metadata/host"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	ddstatus "github.com/DataDog/datadog-agent/pkg/status"
)

func fakeStatusServer(t *testing.T, stats util.Status) *httptest.Server {
	handler := func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		b, err := json.Marshal(stats)
		require.NoError(t, err)

		_, err = w.Write(b)
		require.NoError(t, err)
	}

	return httptest.NewServer(http.HandlerFunc(handler))
}

func TestStatus(t *testing.T) {
	testTime := time.Now()
	expectedStatus := util.Status{
		Date: float64(testTime.UnixNano()),
		Core: util.CoreStatus{
			Metadata: host.Payload{
				Meta: &host.Meta{},
			},
		},
		Expvars: util.ProcessExpvars{},
	}

	server := fakeStatusServer(t, expectedStatus)
	defer server.Close()

	// Build what the expected status should be
	j, err := json.Marshal(expectedStatus)
	require.NoError(t, err)
	expectedOutput, err := ddstatus.FormatProcessAgentStatus(j)
	require.NoError(t, err)

	// Build the actual status
	var statusBuilder strings.Builder
	getAndWriteStatus(server.URL, &statusBuilder, util.OverrideTime(testTime))

	assert.Equal(t, expectedOutput, statusBuilder.String())
}

func TestNotRunning(t *testing.T) {
	// Use different ports in case the host is running a real agent
	cfg := config.Mock()
	cfg.Set("process_config.cmd_port", 8082)

	addressPort, err := api.GetAPIAddressPort()
	require.NoError(t, err)
	statusURL := fmt.Sprintf("http://%s/agent/status", addressPort)

	var b strings.Builder
	getAndWriteStatus(statusURL, &b)

	assert.Equal(t, notRunning, b.String())
}

// TestError tests an example error to make sure that the error template prints properly if we get something other than
// a connection error
func TestError(t *testing.T) {
	cfg := config.Mock()
	cfg.Set("ipc_address", "8.8.8.8") // Non-local ip address will cause error in `GetIPCAddress`
	_, ipcError := config.GetIPCAddress()

	var errText, expectedErrText strings.Builder
	url, err := getStatusURL()
	assert.Equal(t, "", url)
	writeError(&errText, err)

	tpl, err := template.New("").Parse(errorMessage)
	require.NoError(t, err)
	err = tpl.Execute(&expectedErrText, fmt.Errorf("config error: %s", ipcError))
	require.NoError(t, err)

	assert.Equal(t, expectedErrText.String(), errText.String())
}
