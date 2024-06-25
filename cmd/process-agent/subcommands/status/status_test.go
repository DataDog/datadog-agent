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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/process-agent/command"
	hostMetadataUtils "github.com/DataDog/datadog-agent/comp/metadata/host/hostimpl/utils"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/util/status"
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
	statusInfo := status.Status{
		Core: status.CoreStatus{
			Metadata: hostMetadataUtils.Payload{
				Meta: &hostMetadataUtils.Meta{},
			},
		},
		Expvars: status.ProcessExpvars{},
	}

	server := fakeStatusServer(t, statusInfo)
	defer server.Close()

	// Build the actual status
	var statusBuilder strings.Builder
	getAndWriteStatus(log.NoopLogger, server.URL, &statusBuilder)

	expectedOutput := `{"date":0,"core":{"version":"","go_version":"","build_arch":"","config":{"log_level":""},"metadata":{"os":"","agent-flavor":"","python":"","systemStats":null,"meta":{"socket-hostname":"","timezones":null,"socket-fqdn":"","ec2-hostname":"","hostname":"","host_aliases":null,"instance-id":""},"host-tags":null,"network":null,"logs":null,"install-method":null,"proxy-info":null,"otlp":null}},"expvars":{"process_agent":{"pid":0,"uptime":0,"uptime_nano":0,"memstats":{"alloc":0},"version":{"Version":"","GitCommit":"","GitBranch":"","BuildDate":"","GoVersion":""},"docker_socket":"","last_collect_time":"","process_count":0,"container_count":0,"process_queue_size":0,"rtprocess_queue_size":0,"connections_queue_size":0,"event_queue_size":0,"pod_queue_size":0,"process_queue_bytes":0,"rtprocess_queue_bytes":0,"connections_queue_bytes":0,"event_queue_bytes":0,"pod_queue_bytes":0,"container_id":"","proxy_url":"","log_file":"","enabled_checks":null,"endpoints":null,"drop_check_payloads":null,"system_probe_process_module_enabled":false,"language_detection_enabled":false,"workloadmeta_extractor_cache_size":0,"workloadmeta_extractor_stale_diffs":0,"workloadmeta_extractor_diffs_dropped":0}}}`

	assert.Equal(t, expectedOutput, statusBuilder.String())
}

func TestNotRunning(t *testing.T) {
	// Use different ports in case the host is running a real agent
	cfg := config.Mock(t)
	cfg.SetWithoutSource("process_config.cmd_port", 8082)

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
	cfg.SetWithoutSource("cmd_host", "8.8.8.8") // Non-local ip address will cause error in `GetIPCAddress`
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
