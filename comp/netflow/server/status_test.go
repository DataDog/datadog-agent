// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build test

package server

import (
	"bytes"
	"strings"
	"testing"

	nfconfig "github.com/DataDog/datadog-agent/comp/netflow/config"
	"github.com/stretchr/testify/assert"
	"go.uber.org/atomic"
)

func TestStatusProvider(t *testing.T) {
	server := &Server{
		listeners: []*netflowListener{
			{
				flowState: nil,
				config: nfconfig.ListenerConfig{
					BindHost:  "hello",
					FlowType:  "netflow5",
					Namespace: "foo",
				},
				error:     atomic.NewString(""),
				flowCount: atomic.NewInt64(0),
			},
			{
				flowState: nil,
				config: nfconfig.ListenerConfig{
					BindHost:  "world",
					FlowType:  "netflow6",
					Namespace: "bar",
				},
				error:     atomic.NewString("boom"),
				flowCount: atomic.NewInt64(0),
			},
		},
	}

	statusProvider := Provider{
		server: server,
	}

	tests := []struct {
		name       string
		assertFunc func(t *testing.T)
	}{
		{"JSON", func(t *testing.T) {
			stats := make(map[string]interface{})
			statusProvider.JSON(false, stats)

			assert.NotEmpty(t, stats["netflowStats"])
		}},
		{"Text", func(t *testing.T) {
			b := new(bytes.Buffer)
			err := statusProvider.Text(false, b)

			assert.NoError(t, err)

			expectedTextOutput := `
  Total Listeners: 2
  Open Listeners: 1
  Closed Listeners: 1

  === Open Listener Details ===
  ---------
  BindHost: hello
  FlowType: netflow5
  Port: 0
  Workers: 0
  Namespace: foo
  Flows Received: 0
  ---------

  === Closed Listener Details ===
  ---------
  BindHost: world
  FlowType: netflow6
  Port: 0
  Workers: 0
  Namespace: bar
  Error: boom
  ---------
`

			// We replace windows line break by linux so the tests pass on every OS
			expectedResult := strings.Replace(expectedTextOutput, "\r\n", "\n", -1)
			output := strings.Replace(b.String(), "\r\n", "\n", -1)
			assert.Equal(t, expectedResult, output)
		}},
		{"HTML", func(t *testing.T) {
			b := new(bytes.Buffer)
			err := statusProvider.HTML(false, b)

			assert.NoError(t, err)

			expectedHTMLOutput := `<div class="stat">
    <span class="stat_title">NetFlow</span>
    <span class="stat_data">
        Total Listeners: 2
        <br>Open Listeners: 1
        <br>Closed Listeners: 1
        <span class="stat_subtitle">Open Listener Details</span>
        BindHost: hello
        <br>FlowType: netflow5
        <br>Port: 0
        <br>Workers: 0
        <br>Namespace: foo
        <br>Flows Received: 0
        <br>
        <br>
        <br>
        <br>
        <span class="stat_subtitle">Closed Listener Details</span>
        BindHost: world
        <br>FlowType: netflow6
        <br>Port: 0
        <br>Workers: 0
        <br>Namespace: bar
        <br>Error: boom
        <br>
        <br>
    </span>
  </div>`

			expectedResult := strings.Replace(expectedHTMLOutput, "\r\n", "\n", -1)
			output := strings.Replace(b.String(), "\r\n", "\n", -1)
			assert.Equal(t, expectedResult, output)
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.assertFunc(t)
		})
	}
}
