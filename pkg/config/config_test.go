package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewConfig(t *testing.T) {
	c := NewConfig()
	assert.Equal(t, c.DdURL, "https://app.datadoghq.com")
	assert.Equal(t, c.ForwarderTimeout, 20)
	assert.Equal(t, c.RecentPointThreshold, 30)
	assert.Equal(t, c.HistogramAggregates, "max, median, avg, count")
	assert.Equal(t, c.HistogramPercentiles, 0.95)
	assert.Equal(t, c.ProcfsPath, "/proc")
}

func TestFromYAML(t *testing.T) {
	fixture := `
  dd_url: https://app.datadoghq.com
  proxy_host: my-proxy.com
  proxy_port: 3128
  api_key: abcdefghijklmnopqrstuvwxyz1234567890
  forwarder_timeout: 19
  recent_point_threshold: 29
  histogram_aggregates: foo, bar, baz
  histogram_percentiles: 0.94
  procfs_path: /foo/bar
  does_not_exist: foo
  `
	c := NewConfig()
	err := c.FromYAML([]byte(fixture))
	assert.Nil(t, err)
	assert.Equal(t, c.ProxyHost, "my-proxy.com")
	assert.Equal(t, c.ProxyPort, 3128)
	assert.Equal(t, c.APIKey, "abcdefghijklmnopqrstuvwxyz1234567890")
	assert.Equal(t, c.ForwarderTimeout, 19)
	assert.Equal(t, c.RecentPointThreshold, 29)
	assert.Equal(t, c.HistogramAggregates, "foo, bar, baz")
	assert.Equal(t, c.HistogramPercentiles, 0.94)
	assert.Equal(t, c.ProcfsPath, "/foo/bar")

	err = c.FromYAML([]byte("/"))
	assert.NotNil(t, err)
}
