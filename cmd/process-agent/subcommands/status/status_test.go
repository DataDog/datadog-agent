// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package status

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"text/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/process-agent/command"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func fakeStatusServer(t *testing.T, ipcMock *ipcmock.IPCMock, body string) *httptest.Server {
	handler := func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		_, err := w.Write([]byte(body))
		require.NoError(t, err)
	}

	return ipcMock.NewMockServer(http.HandlerFunc(handler))
}

func TestStatus(t *testing.T) {
	const fakeBody = "process-agent status response body"

	ipcMock := ipcmock.New(t)
	server := fakeStatusServer(t, ipcMock, fakeBody)

	var statusBuilder strings.Builder
	getAndWriteStatus(log.NoopLogger, ipcMock.GetClient(), server.URL, &statusBuilder)

	assert.Equal(t, fakeBody, statusBuilder.String())
}

func TestNotRunning(t *testing.T) {
	// Use different ports in case the host is running a real agent
	cfg := configmock.New(t)
	cfg.SetWithoutSource("process_config.cmd_port", 8082)

	addressPort, err := pkgconfigsetup.GetProcessAPIAddressPort(cfg)
	require.NoError(t, err)
	statusURL := fmt.Sprintf("https://%s/agent/status", addressPort)

	ipcMock := ipcmock.New(t)

	var b strings.Builder
	getAndWriteStatus(log.NoopLogger, ipcMock.GetClient(), statusURL, &b)

	assert.Equal(t, notRunning, b.String())
}

// TestError tests an example error to make sure that the error template prints properly if we get something other than
// a connection error
func TestError(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetWithoutSource("cmd_host", "8.8.8.8") // Non-local ip address will cause error in `GetIPCAddress`
	_, ipcError := pkgconfigsetup.GetIPCAddress(cfg)

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
