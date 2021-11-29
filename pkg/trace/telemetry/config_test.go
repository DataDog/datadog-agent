package telemetry

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/stretchr/testify/assert"
)

// mockConfig is a single config entry mocking util with automatic reset
//
// Usage: defer mockConfig(key, new_value)()
// will automatically revert previous once current scope exits
func mockConfig(k string, v interface{}) func() {
	oldConfig := config.Datadog
	config.Mock().Set(k, v)
	return func() { config.Datadog = oldConfig }
}

func TestTelemetryProxyTargetsBuild(t *testing.T) {
	t.Run("default config", func(t *testing.T) {
		target, err := BuildBaseTarget("")
		assert.NoError(t, err)
		assert.Equal(t, "https://instrumentation-telemetry-intake.datadoghq.com/", target.url.String())

		targets := BuildAdditionalTargets()
		assert.Empty(t, targets)
	})

	t.Run("changed dd site", func(t *testing.T) {
		defer mockConfig("site", "new_site.example.com")()

		target, err := BuildBaseTarget("")
		assert.NoError(t, err)
		assert.Equal(t, "https://instrumentation-telemetry-intake.new_site.example.com/", target.url.String())

		targets := BuildAdditionalTargets()
		assert.Empty(t, targets)
	})

	t.Run("default site with additional endpoints", func(t *testing.T) {
		additionalEndpoints := make(map[string]string)
		additionalEndpoints["http://test_backend_2.example.com/"] = "test_apikey_2"
		additionalEndpoints["http://test_backend_3.example.com"] = "test_apikey_3"

		defer mockConfig("apm_config.telemetry.additional_endpoints", additionalEndpoints)()

		target, err := BuildBaseTarget("")
		assert.NoError(t, err)
		assert.Equal(t, "https://instrumentation-telemetry-intake.datadoghq.com/", target.url.String())

		targets := BuildAdditionalTargets()
		assert.Len(t, targets, 2)

		for _, target := range targets {
			assert.NotNil(t, additionalEndpoints[target.url.String()])
			assert.Equal(t, target.apiKey, additionalEndpoints[target.url.String()])
		}
	})

	t.Run("default site with malformed additional endpoints", func(t *testing.T) {
		additionalEndpoints := make(map[string]string)
		additionalEndpoints["11://test_backend_2.example.com///"] = "test_apikey_2"
		additionalEndpoints["http://test_backend_3.example.com"] = "test_apikey_3"

		defer mockConfig("apm_config.telemetry.additional_endpoints", additionalEndpoints)()
		target, err := BuildBaseTarget("")
		assert.NoError(t, err)
		assert.Equal(t, "https://instrumentation-telemetry-intake.datadoghq.com/", target.url.String())

		targets := BuildAdditionalTargets()

		assert.Len(t, targets, 1)
	})

	t.Run("dd_url malformed causes error", func(t *testing.T) {
		defer mockConfig("apm_config.telemetry.dd_url", "111://abc.com")()

		target, err := BuildBaseTarget("")
		assert.Error(t, err)
		assert.Nil(t, target)

		assert.Empty(t, BuildAdditionalTargets())
	})
}
