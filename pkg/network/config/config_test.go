package config

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/stretchr/testify/assert"
)

var originalConfig = config.Datadog

func restoreGlobalConfig() {
	config.Datadog = originalConfig
}

func newConfig() {
	config.Datadog = config.NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))
	config.InitConfig(config.Datadog)
}

func TestDisablingDNSInspection(t *testing.T) {
	t.Run("via YAML", func(t *testing.T) {
		newConfig()
		defer restoreGlobalConfig()
		_, err := sysconfig.New("./testdata/TestDDAgentConfigYamlAndSystemProbeConfig-DisableDNS.yaml")
		require.NoError(t, err)
		cfg := New()

		assert.False(t, cfg.DNSInspection)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		newConfig()
		defer restoreGlobalConfig()

		os.Setenv("DD_DISABLE_DNS_INSPECTION", "true")
		defer os.Unsetenv("DD_DISABLE_DNS_INSPECTION")
		_, err := sysconfig.New("")
		require.NoError(t, err)
		cfg := New()

		assert.False(t, cfg.DNSInspection)
	})
}

func TestEnableHTTPMonitoring(t *testing.T) {
	t.Run("via YAML", func(t *testing.T) {
		newConfig()
		defer restoreGlobalConfig()

		_, err := sysconfig.New("./testdata/TestDDAgentConfigYamlAndSystemProbeConfig-EnableHTTP.yaml")
		require.NoError(t, err)
		cfg := New()

		assert.True(t, cfg.EnableHTTPMonitoring)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		newConfig()
		defer restoreGlobalConfig()

		os.Setenv("DD_SYSTEM_PROBE_NETWORK_ENABLE_HTTP_MONITORING", "true")
		defer os.Unsetenv("DD_SYSTEM_PROBE_NETWORK_ENABLE_HTTP_MONITORING")
		_, err := sysconfig.New("")
		require.NoError(t, err)
		cfg := New()

		assert.True(t, cfg.EnableHTTPMonitoring)
	})
}

func TestEnableGatewayLookup(t *testing.T) {
	t.Run("via YAML", func(t *testing.T) {
		newConfig()
		defer restoreGlobalConfig()

		// default config
		_, err := sysconfig.New("")
		require.NoError(t, err)
		cfg := New()

		assert.False(t, cfg.EnableGatewayLookup)

		newConfig()
		_, err = sysconfig.New("./testdata/TestDDAgentConfigYamlAndSystemProbeConfig-EnableGwLookup.yaml")
		require.NoError(t, err)
		cfg = New()

		assert.True(t, cfg.EnableGatewayLookup)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		newConfig()
		defer restoreGlobalConfig()

		os.Setenv("DD_SYSTEM_PROBE_NETWORK_ENABLE_GATEWAY_LOOKUP", "true")
		defer os.Unsetenv("DD_SYSTEM_PROBE_NETWORK_ENABLE_GATEWAY_LOOKUP")
		_, err := sysconfig.New("")
		require.NoError(t, err)
		cfg := New()

		assert.True(t, cfg.EnableGatewayLookup)
	})
}

func TestIgnoreConntrackInitFailure(t *testing.T) {
	t.Run("via YAML", func(t *testing.T) {
		newConfig()
		defer restoreGlobalConfig()

		_, err := sysconfig.New("./testdata/TestDDAgentConfigYamlAndSystemProbeConfig-IgnoreCTInitFailure.yaml")
		require.NoError(t, err)
		cfg := New()

		assert.True(t, cfg.IgnoreConntrackInitFailure)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		newConfig()
		defer restoreGlobalConfig()

		os.Setenv("DD_SYSTEM_PROBE_NETWORK_IGNORE_CONNTRACK_INIT_FAILURE", "true")
		defer os.Unsetenv("DD_SYSTEM_PROBE_NETWORK_IGNORE_CONNTRACK_INIT_FAILURE")
		_, err := sysconfig.New("")
		require.NoError(t, err)
		cfg := New()

		assert.Nil(t, err)
		assert.True(t, cfg.IgnoreConntrackInitFailure)
	})
}

func TestEnablingDNSStatsCollection(t *testing.T) {
	newConfig()
	defer restoreGlobalConfig()

	t.Run("via YAML", func(t *testing.T) {
		_, err := sysconfig.New("./testdata/TestDDAgentConfigYamlAndSystemProbeConfig-EnableDNSStats.yaml")
		require.NoError(t, err)
		cfg := New()

		assert.True(t, cfg.CollectDNSStats)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		defer os.Unsetenv("DD_COLLECT_DNS_STATS")

		os.Setenv("DD_COLLECT_DNS_STATS", "false")
		_, err := sysconfig.New("")
		require.NoError(t, err)
		cfg := New()

		assert.False(t, cfg.CollectDNSStats)

		newConfig()
		os.Setenv("DD_COLLECT_DNS_STATS", "true")
		_, err = sysconfig.New("")
		require.NoError(t, err)
		cfg = New()

		assert.True(t, cfg.CollectDNSStats)
	})
}

func TestEnablingDNSDomainCollection(t *testing.T) {
	newConfig()
	defer restoreGlobalConfig()

	t.Run("via YAML", func(t *testing.T) {
		_, err := sysconfig.New("./testdata/TestDDAgentConfigYamlAndSystemProbeConfig-EnableDNSDomains.yaml")
		require.NoError(t, err)
		cfg := New()

		assert.True(t, cfg.CollectDNSDomains)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		defer os.Unsetenv("DD_COLLECT_DNS_DOMAINS")

		os.Setenv("DD_COLLECT_DNS_DOMAINS", "false")
		_, err := sysconfig.New("")
		require.NoError(t, err)
		cfg := New()

		assert.False(t, cfg.CollectDNSDomains) // default value should be false

		newConfig()
		os.Setenv("DD_COLLECT_DNS_DOMAINS", "true")
		_, err = sysconfig.New("")
		require.NoError(t, err)
		cfg = New()

		assert.True(t, cfg.CollectDNSDomains)
	})
}

func TestSettingMaxDNSStats(t *testing.T) {
	newConfig()
	defer restoreGlobalConfig()

	t.Run("via YAML", func(t *testing.T) {
		_, err := sysconfig.New("./testdata/TestDDAgentConfigYamlAndSystemProbeConfig-EnableDNSDomains.yaml")
		require.NoError(t, err)
		cfg := New()

		assert.Equal(t, 100, cfg.MaxDNSStats)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		newConfig()
		os.Unsetenv("DD_SYSTEM_PROBE_CONFIG_MAX_DNS_STATS")
		_, err := sysconfig.New("")
		require.NoError(t, err)
		cfg := New()

		assert.Equal(t, 20000, cfg.MaxDNSStats) // default value

		newConfig()
		os.Setenv("DD_SYSTEM_PROBE_CONFIG_MAX_DNS_STATS", "10000")
		_, err = sysconfig.New("")
		require.NoError(t, err)
		cfg = New()

		assert.Equal(t, 10000, cfg.MaxDNSStats)
	})
}
