// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package status

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

	"github.com/DataDog/datadog-agent/cmd/process-agent/command"
	hostMetadataUtils "github.com/DataDog/datadog-agent/comp/metadata/host/utils"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/util/status"
	ddstatus "github.com/DataDog/datadog-agent/pkg/status"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func fakeStatusServer(t *testing.T, stats status.Status) *httptest.Server {
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
	expectedStatus := status.Status{
		Date: float64(testTime.UnixNano()),
		Core: status.CoreStatus{
			Metadata: hostMetadataUtils.Payload{
				Meta: &hostMetadataUtils.Meta{},
			},
		},
		Expvars: status.ProcessExpvars{},
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
	getAndWriteStatus(log.NoopLogger, server.URL, &statusBuilder, status.OverrideTime(testTime))

	assert.Equal(t, expectedOutput, statusBuilder.String())
}

func TestNotRunning(t *testing.T) {
	// Use different ports in case the host is running a real agent
	cfg := config.Mock(t)
	cfg.Set("process_config.cmd_port", 8082)

	addressPort, err := config.GetProcessAPIAddressPort()
	require.NoError(t, err)
	statusURL := fmt.Sprintf("http://%s/agent/status", addressPort)

	var b strings.Builder
	getAndWriteStatus(log.NoopLogger, statusURL, &b)

	assert.Equal(t, notRunning, b.String())
}

// TestError tests an example error to make sure that the error template prints properly if we get something other than
// a connection error
func TestError(t *testing.T) {
	cfg := config.Mock(t)
	cfg.Set("ipc_address", "8.8.8.8") // Non-local ip address will cause error in `GetIPCAddress`
	_, ipcError := config.GetIPCAddress()

	var errText, expectedErrText strings.Builder
	url, err := getStatusURL()
	assert.Equal(t, "", url)
	writeError(log.NoopLogger, &errText, err)

	tpl, err := template.New("").Parse(errorMessage)
	require.NoError(t, err)
	err = tpl.Execute(&expectedErrText, fmt.Errorf("config error: %s", ipcError))
	require.NoError(t, err)

	assert.Equal(t, expectedErrText.String(), errText.String())
}

func TestRunStatusCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"status"},
		runStatus,
		func() {})
}
