// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package statusimpl

import (
	"bytes"
	"expvar"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	_ "github.com/DataDog/datadog-agent/pkg/aggregator"
)

func TestStatusProvider(t *testing.T) {
	provider := Provider{}

	// Set SNMP Traps errors
	aggregatorMetrics, ok := expvar.Get("aggregator").(*expvar.Map)
	require.True(t, ok)
	epErrors, ok := aggregatorMetrics.Get("EventPlatformEventsErrors").(*expvar.Map)
	require.True(t, ok)
	epErrors.Add(eventplatform.EventTypeSnmpTraps, 42)
	require.True(t, ok)

	tests := []struct {
		name       string
		assertFunc func(t *testing.T)
	}{
		{"JSON", func(t *testing.T) {
			stats := make(map[string]interface{})
			provider.JSON(false, stats)

			snmpStatus := stats["snmpTrapsStats"]

			assert.NotEmpty(t, snmpStatus)

			snmpStatusMap := snmpStatus.(map[string]interface{})
			metrics := snmpStatusMap["metrics"].(map[string]interface{})

			// assert packets is float64
			_ = metrics["Packets"].(float64)
			// assert PacketsDropped is float64
			_ = metrics["PacketsDropped"].(float64)
			// assert PacketsUnknownCommunityString is float64
			_ = metrics["PacketsUnknownCommunityString"].(float64)
		}},
		{"Text", func(t *testing.T) {
			b := new(bytes.Buffer)
			err := provider.Text(false, b)

			assert.NoError(t, err)

			expectedOutput := `
  Packets: 0
  Packets Dropped: 42
  Packets Unknown Community String: 0
`

			// We replace windows line break by linux so the tests pass on every OS
			expectedResult := strings.Replace(expectedOutput, "\r\n", "\n", -1)
			output := strings.Replace(b.String(), "\r\n", "\n", -1)

			assert.Equal(t, expectedResult, output)
		}},
		{"HTML", func(t *testing.T) {
			b := new(bytes.Buffer)
			err := provider.HTML(false, b)

			assert.NoError(t, err)

			expectedOutput := `
  <div class="stat">
    <span class="stat_title">SNMP Traps</span>
    <span class="stat_data">
          Packets: 0<br>
          Packets Dropped: 42<br>
          Packets Unknown Community String: 0<br>
    </span>
  </div>
`

			// We replace windows line break by linux so the tests pass on every OS
			expectedResult := strings.Replace(expectedOutput, "\r\n", "\n", -1)
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
