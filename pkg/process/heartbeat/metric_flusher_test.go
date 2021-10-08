// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build test

package heartbeat

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var then = time.Unix(123, 0)
var requestBody = `
{
  "series": [
    {
      "metric": "datadog.system_probe.agent.network_tracer",
      "points": [[123, 1]],
      "tags": ["version:8", "revision:123"],
      "host": "foobar",
      "type": "gauge",
      "interval": 0
    },
    {
      "metric": "datadog.system_probe.agent.oom_kill_probe",
      "points": [[123, 1]],
      "tags": ["version:8", "revision:123"],
      "host": "foobar",
      "type": "gauge",
      "interval": 0
    }
  ]
}
`

func TestFlush(t *testing.T) {
	mockForwarder := &forwarder.MockedForwarder{}
	tags := []string{"version:8", "revision:123"}
	flusher := &apiFlusher{
		forwarder:  mockForwarder,
		hostname:   "foobar",
		tags:       tags,
		apiWatcher: newAPIWatcher(time.Minute),
	}
	var payloads forwarder.Payloads

	mockForwarder.
		On("SubmitV1Series", mock.AnythingOfType("forwarder.Payloads"), mock.AnythingOfType("http.Header")).
		Return(nil).
		Times(1).
		Run(func(args mock.Arguments) {
			payloads = args.Get(0).(forwarder.Payloads)
		})

	flusher.Flush([]string{"datadog.system_probe.agent.network_tracer", "datadog.system_probe.agent.oom_kill_probe"}, then)
	mockForwarder.AssertExpectations(t)
	assert.JSONEq(t, requestBody, string(*payloads[0]))
}

func TestURLSanitization(t *testing.T) {
	type testCase struct {
		originalURL string
		expectedURL string
	}

	testCases := []testCase{
		{
			originalURL: "https://process.datadoghq.com",
			expectedURL: "https://app.datadoghq.com",
		},
		{
			originalURL: "process.datadoghq.com",
			expectedURL: "https://app.datadoghq.com",
		},
		{
			originalURL: "https://process.datad0g.com",
			expectedURL: "https://app.datad0g.com",
		},
		{
			originalURL: "https://k8s.process.datad0g.com",
			expectedURL: "https://app.datad0g.com",
		},
		{
			originalURL: "https://process.datadoghq.eu",
			expectedURL: "https://app.datadoghq.eu",
		},
	}

	for _, tc := range testCases {
		keysPerDomain := map[string][]string{
			tc.originalURL: {"dd-api-key"},
		}

		result, err := sanitize(keysPerDomain)
		assert.Nil(t, err)
		assert.Len(t, result, 1)
		for url, keys := range result {
			assert.Equal(t, tc.expectedURL, url)
			assert.Equal(t, keys, []string{"dd-api-key"})
		}
	}
}
