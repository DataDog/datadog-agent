// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package statusimpl

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var jsonResponse = `{
	"core": {
		"build_arch": "arm64",
		"config": {
			"log_level": "info"
		},
		"go_version": "go1.21.5",
		"metadata": {
			"agent-flavor": "process_agent",
			"host-tags": {
				"system": []
			},
			"install-method": {
				"installer_version": null,
				"tool": null,
				"tool_version": "undefined"
			},
			"logs": {
				"auto_multi_line_detection_enabled": false,
				"transport": ""
			},
			"meta": {
				"ec2-hostname": "",
				"host_aliases": [],
				"hostname": "COMP-VQHPF4W6GY",
				"instance-id": "",
				"socket-fqdn": "COMP-VQHPF4W6GY",
				"socket-hostname": "COMP-VQHPF4W6GY",
				"timezones": [
					"CET"
				]
			},
			"network": null,
			"os": "darwin",
			"otlp": {
				"enabled": false
			},
			"proxy-info": {
				"no-proxy-nonexact-match": false,
				"no-proxy-nonexact-match-explicitly-set": false,
				"proxy-behavior-changed": false
			},
			"python": "n/a",
			"systemStats": {
				"cpuCores": 10,
				"fbsdV": [
					"",
					"",
					""
				],
				"macV": [
					"14.2.1",
					[
						"",
						"",
						""
					],
					"arm64"
				],
				"machine": "arm64",
				"nixV": [
					"",
					"",
					""
				],
				"platform": "darwin",
				"processor": "Apple M1 Max",
				"pythonV": "n/a",
				"winV": [
					"",
					"",
					""
				]
			}
		},
		"version": "7.51.0-rc.1+git.416.0d1edc1"
	},
	"date": 1706892483712089000,
	"expvars": {
		"connections_queue_bytes": 0,
		"connections_queue_size": 0,
		"container_count": 0,
		"container_id": "",
		"docker_socket": "/var/run/docker.sock",
		"drop_check_payloads": [],
		"enabled_checks": [
			"process",
			"rtprocess"
		],
		"endpoints": {
			"https://process.datadoghq.eu": [
				"72724"
			]
		},
		"event_queue_bytes": 0,
		"event_queue_size": 0,
		"language_detection_enabled": false,
		"last_collect_time": "2024-02-02 17:47:57",
		"log_file": "",
		"memstats": {
			"alloc": 30387880
		},
		"pid": 72211,
		"pod_queue_bytes": 0,
		"pod_queue_size": 0,
		"process_count": 757,
		"process_queue_bytes": 0,
		"process_queue_size": 0,
		"proxy_url": "",
		"rtprocess_queue_bytes": 0,
		"rtprocess_queue_size": 0,
		"system_probe_process_module_enabled": false,
		"uptime": 18,
		"uptime_nano": 1706892464835469000,
		"version": {
			"BuildDate": "",
			"GitBranch": "",
			"GitCommit": "",
			"GoVersion": "",
			"Version": ""
		},
		"workloadmeta_extractor_cache_size": 0,
		"workloadmeta_extractor_diffs_dropped": 0,
		"workloadmeta_extractor_stale_diffs": 0
	}
}
`

var textResponse = `
  Version: 7.51.0-rc.1+git.416.0d1edc1
  Status date: 2024-02-02 17:48:03.712 CET / 2024-02-02 16:48:03.712 UTC (1706892483712)
  Process Agent Start: 2024-02-02 17:47:44.835 CET / 2024-02-02 16:47:44.835 UTC (1706892464835)
  Pid: 72211
  Go Version: go1.21.5
  Build arch: arm64
  Log Level: info
  Enabled Checks: [process rtprocess]
  Allocated Memory: 30,387,880 bytes
  Hostname: COMP-VQHPF4W6GY
  System Probe Process Module Status: Not running
  Process Language Detection Enabled: False

  =================
  Process Endpoints
  =================
    https://process.datadoghq.eu - API Key ending with:
        - 72724

  =========
  Collector
  =========
    Last collection time: 2024-02-02 17:47:57
    Docker socket: /var/run/docker.sock
    Number of processes: 757
    Number of containers: 0
    Process Queue length: 0
    RTProcess Queue length: 0
    Connections Queue length: 0
    Event Queue length: 0
    Pod Queue length: 0
    Process Bytes enqueued: 0
    RTProcess Bytes enqueued: 0
    Connections Bytes enqueued: 0
    Event Bytes enqueued: 0
    Pod Bytes enqueued: 0
    Drop Check Payloads: []

  ==========
  Extractors
  ==========

    Workloadmeta
    ============
      Cache size: 0
      Stale diffs discarded: 0
      Diffs dropped: 0
`

func fakeStatusServer(t *testing.T, errCode int, response string) *httptest.Server {
	handler := func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		if errCode != 200 {
			http.NotFound(w, r)
		} else {
			_, err := w.Write([]byte(response))
			require.NoError(t, err)
		}
	}

	return httptest.NewServer(http.HandlerFunc(handler))
}

func TestStatus(t *testing.T) {
	server := fakeStatusServer(t, 200, jsonResponse)
	defer server.Close()

	headerProvider := statusProvider{
		testServerURL: server.URL,
	}

	tests := []struct {
		name       string
		assertFunc func(t *testing.T)
	}{
		{"JSON", func(t *testing.T) {
			stats := make(map[string]interface{})
			headerProvider.JSON(false, stats)
			processStats := stats["processAgentStatus"]

			val, ok := processStats.(map[string]interface{})
			assert.True(t, ok)

			assert.NotEmpty(t, val["core"])
			assert.Empty(t, val["error"])
		}},
		{"Text", func(t *testing.T) {
			b := new(bytes.Buffer)
			err := headerProvider.Text(false, b)

			assert.NoError(t, err)

			assert.Equal(t, textResponse, b.String())
		}},
		{"HTML", func(t *testing.T) {
			b := new(bytes.Buffer)
			err := headerProvider.HTML(false, b)

			assert.NoError(t, err)

			assert.Empty(t, b.String())
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.assertFunc(t)
		})
	}
}

func TestStatusError(t *testing.T) {
	server := fakeStatusServer(t, 500, "")
	defer server.Close()

	headerProvider := statusProvider{
		testServerURL: server.URL,
	}

	tests := []struct {
		name       string
		assertFunc func(t *testing.T)
	}{
		{"JSON", func(t *testing.T) {
			stats := make(map[string]interface{})
			headerProvider.JSON(false, stats)
			processStats := stats["processAgentStatus"]

			val, ok := processStats.(map[string]interface{})
			assert.True(t, ok)

			assert.NotEmpty(t, val["error"])
		}},
		{"Text", func(t *testing.T) {
			b := new(bytes.Buffer)
			err := headerProvider.Text(false, b)

			assert.NoError(t, err)

			assert.Equal(t, "\n  Status: Not running or unreachable\n", b.String())
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.assertFunc(t)
		})
	}
}
