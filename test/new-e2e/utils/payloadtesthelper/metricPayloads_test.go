package payloadtesthelper

import (
	_ "embed"
	"testing"

	"github.com/stretchr/testify/assert"
)

//go:embed fixtures/api_v2_series_dump.txt
var metricsBody []byte

func TestNewMetricPayloads(t *testing.T) {
	t.Run("UnmarshallPayloads", func(t *testing.T) {
		mp := NewMetricPayloads()
		err := mp.UnmarshallPayloads(metricsBody)
		assert.NoError(t, err)
		assert.Equal(t, 74, len(mp.metricsByName))
	})

	t.Run("ContainsMetricName", func(t *testing.T) {
		mp := NewMetricPayloads()
		err := mp.UnmarshallPayloads(metricsBody)
		assert.NoError(t, err)
		assert.True(t, mp.ContainsMetricName("system.uptime"))
		assert.False(t, mp.ContainsMetricName("invalid.name"))
	})

	t.Run("ContainsMetricName", func(t *testing.T) {
		mp := NewMetricPayloads()
		err := mp.UnmarshallPayloads(metricsBody)
		assert.NoError(t, err)
		assert.True(t, mp.ContainsMetricNameAndTags("datadog.agent.python.version", []string{"python_version:3", "agent_version_major:7"}))
		assert.False(t, mp.ContainsMetricNameAndTags("datadog.agent.python.version", []string{"python_version:3", "invalid:tag"}))
	})
}
