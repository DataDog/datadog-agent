// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package statusimpl

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	"github.com/DataDog/datadog-agent/comp/core/status"
	pbcore "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

func TestStatusOut(t *testing.T) {
	reqs := Requires{
		Config: config.NewMock(t),
		Client: ipcmock.New(t).GetClient(),
	}

	provides := NewComponent(reqs)

	headerProvider := provides.StatusProvider.Provider

	tests := []struct {
		name       string
		assertFunc func(t *testing.T)
	}{
		{"JSON", func(t *testing.T) {
			stats := make(map[string]interface{})
			headerProvider.JSON(false, stats)

			assert.NotEmpty(t, stats)
		}},
		{"Text", func(t *testing.T) {
			b := new(bytes.Buffer)
			err := headerProvider.Text(false, b)

			assert.NoError(t, err)

			assert.NotEmpty(t, b.String())
		}},
		{"HTML", func(t *testing.T) {
			b := new(bytes.Buffer)
			err := headerProvider.HTML(false, b)

			assert.NoError(t, err)

			assert.NotEmpty(t, b.String())
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.assertFunc(t)
		})
	}
}

func TestSemanticCoreRendered(t *testing.T) {
	stats := map[string]interface{}{
		"apmStats": map[string]interface{}{
			"pid":                    "123",
			"uptime":                 10,
			"memstats":               map[string]interface{}{"Alloc": float64(1024)},
			"config":                 map[string]interface{}{"Hostname": "h", "ReceiverHost": "localhost", "ReceiverPort": float64(8126), "Endpoints": []interface{}{}},
			"receiver":               []interface{}{},
			"ratebyservice_filtered": map[string]interface{}{},
			"trace_writer":           map[string]interface{}{"Payloads": float64(0), "Traces": float64(0), "Events": float64(0), "Bytes": float64(0), "Errors": float64(0)},
			"stats_writer":           map[string]interface{}{"Payloads": float64(0), "StatsBuckets": float64(0), "Bytes": float64(0), "Errors": float64(0)},
			"trace_semantics": map[string]interface{}{
				"Source":      "remote-config",
				"ContentHash": "hash-rc",
				"Version":     "rc-1.0",
			},
		},
	}

	b := new(bytes.Buffer)
	require.NoError(t, status.RenderText(templatesFS, "traceagent.tmpl", b, stats))
	out := b.String()
	assert.Contains(t, out, "Trace Semantics")
	assert.Contains(t, out, "Source: Remote Config")
	assert.Contains(t, out, "hash-rc")
	assert.Contains(t, out, "rc-1.0")
}

func TestGetStatusDetailsMatchesText(t *testing.T) {
	ipc := ipcmock.New(t)
	server := ipc.NewMockServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, err := w.Write([]byte(`{
			"pid": 123,
			"uptime": 10,
			"memstats": {"Alloc": 1024},
			"config": {
				"Hostname": "trace-host",
				"ReceiverHost": "localhost",
				"ReceiverPort": 8126,
				"Endpoints": [{"Host": "https://trace.agent.example"}]
			},
			"receiver": [],
			"ratebyservice_filtered": {},
			"trace_writer": {"Payloads": 2, "Traces": 3, "Events": 4, "Bytes": 1024, "Errors": 0},
			"stats_writer": {"Payloads": 5, "StatsBuckets": 6, "Bytes": 2048, "Errors": 0},
			"trace_semantics": {"Source": "remote-config", "ContentHash": "hash-rc", "Version": "rc-1.0"}
		}`))
		assert.NoError(t, err)
	}))

	port := server.Listener.Addr().(*net.TCPAddr).Port

	configComponent := config.NewMock(t)
	configComponent.SetInTest("apm_config.debug.port", port)
	configComponent.SetInTest("server_timeout", 1)

	provides := NewComponent(Requires{
		Config: configComponent,
		Client: ipc.GetClient(),
	})
	require.Same(t, provides.Comp, provides.StatusProvider.Provider)

	var expected bytes.Buffer
	require.NoError(t, provides.StatusProvider.Provider.Text(false, &expected))

	response, err := provides.Comp.GetStatusDetails(context.Background(), &pbcore.GetStatusDetailsRequest{})
	require.NoError(t, err)
	require.Contains(t, response.NamedSections, "Details")
	assert.Equal(t, expected.String(), response.NamedSections["Details"].Fields[""])
}
