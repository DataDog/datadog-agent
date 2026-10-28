// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package statusimpl

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	"github.com/DataDog/datadog-agent/comp/core/status"
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

type recordingClient struct {
	ipc.HTTPClient
	url string
}

func (c *recordingClient) Get(rawURL string, _ ...ipc.RequestOption) ([]byte, error) {
	c.url = rawURL
	return []byte(`{}`), nil
}

func TestPopulateStatusUsesCmdHost(t *testing.T) {
	cfg := config.NewMockWithOverrides(t, map[string]interface{}{
		"cmd_host":              "127.0.0.1",
		"apm_config.debug.port": 5012,
	})
	client := &recordingClient{}
	provider := statusProvider{Config: cfg, Client: client}

	provider.populateStatus()
	assert.Equal(t, "https://127.0.0.1:5012/debug/vars", client.url)
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
