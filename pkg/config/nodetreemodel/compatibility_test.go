// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package nodetreemodel defines a model for the config using a tree of nodes
package nodetreemodel

import (
	"bytes"
	"maps"
	"slices"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/viperconfig"
	"github.com/stretchr/testify/assert"
)

func constructBothConfigs(content string, dynamicSchema bool, setupFunc func(model.Setup)) (model.Config, model.Config) {
	viperConf := viperconfig.NewViperConfig("datadog", "DD", strings.NewReplacer(".", "_")) // nolint: forbidigo // legit use case
	ntmConf := NewNodeTreeConfig("datadog", "DD", strings.NewReplacer(".", "_"))            // nolint: forbidigo // legit use case

	if dynamicSchema {
		viperConf.SetTestOnlyDynamicSchema(true)
		ntmConf.SetTestOnlyDynamicSchema(true)
	}
	if setupFunc != nil {
		setupFunc(viperConf)
		setupFunc(ntmConf)
	}

	viperConf.BuildSchema()
	ntmConf.BuildSchema()

	if len(content) > 0 {
		viperConf.SetConfigType("yaml")
		viperConf.ReadConfig(bytes.NewBuffer([]byte(content)))

		ntmConf.SetConfigType("yaml")
		ntmConf.ReadConfig(bytes.NewBuffer([]byte(content)))
	}

	return viperConf, ntmConf
}

func TestCompareGetInt(t *testing.T) {
	dataYaml := `port: 345`
	viperConf, ntmConf := constructBothConfigs(dataYaml, true, nil)

	assert.Equal(t, 345, viperConf.GetInt("port"))
	assert.Equal(t, 345, ntmConf.GetInt("port"))

	viperConf, ntmConf = constructBothConfigs(dataYaml, false, func(cfg model.Setup) {
		cfg.SetKnown("port")
	})
	assert.Equal(t, 345, viperConf.GetInt("port"))
	assert.Equal(t, 345, ntmConf.GetInt("port"))
}

func TestCompareIsSet(t *testing.T) {
	dataYaml := `port: 345`
	viperConf, ntmConf := constructBothConfigs(dataYaml, true, nil)
	assert.Equal(t, true, viperConf.IsSet("port"))
	assert.Equal(t, true, ntmConf.IsSet("port"))

	viperConf, ntmConf = constructBothConfigs(dataYaml, false, func(cfg model.Setup) {
		cfg.SetKnown("port")
	})
	assert.Equal(t, true, viperConf.IsSet("port"))
	assert.Equal(t, true, ntmConf.IsSet("port"))

	dataYaml = ``
	viperConf, ntmConf = constructBothConfigs(dataYaml, false, func(cfg model.Setup) {
		cfg.SetDefault("port", 123)
	})
	assert.Equal(t, 123, viperConf.GetInt("port"))
	assert.Equal(t, 123, ntmConf.GetInt("port"))
	assert.Equal(t, true, viperConf.IsSet("port"))
	assert.Equal(t, true, ntmConf.IsSet("port"))

	t.Setenv("TEST_PORT", "789")
	viperConf, ntmConf = constructBothConfigs(dataYaml, false, func(cfg model.Setup) {
		cfg.BindEnv("port", "TEST_PORT")
	})
	assert.Equal(t, 789, viperConf.GetInt("port"))
	assert.Equal(t, 789, ntmConf.GetInt("port"))
	assert.Equal(t, true, viperConf.IsSet("port"))
	assert.Equal(t, true, ntmConf.IsSet("port"))

	t.Setenv("TEST_PORT", "")
	viperConf, ntmConf = constructBothConfigs(dataYaml, false, func(cfg model.Setup) {
		cfg.BindEnv("port", "TEST_PORT")
	})
	assert.Equal(t, 0, viperConf.GetInt("port"))
	assert.Equal(t, 0, ntmConf.GetInt("port"))
	assert.Equal(t, false, viperConf.IsSet("port"))
	assert.Equal(t, false, ntmConf.IsSet("port"))
}

func TestCompareAllSettingsWithoutDefault(t *testing.T) {
	dataYaml := `additional_endpoints:
  0: apple
  1: banana
  2: cherry
`
	viperConf, ntmConf := constructBothConfigs(dataYaml, true, nil)

	expectedYaml := `additional_endpoints:
  "0": apple
  "1": banana
  "2": cherry
`
	yamlConf, err := yaml.Marshal(viperConf.AllSettingsWithoutDefault())
	assert.NoError(t, err)
	yamlText := string(yamlConf)
	assert.Equal(t, expectedYaml, yamlText)

	expectedYaml = `additional_endpoints:
  "0": apple
  "1": banana
  "2": cherry
`
	yamlConf, err = yaml.Marshal(ntmConf.AllSettingsWithoutDefault())
	assert.NoError(t, err)
	yamlText = string(yamlConf)
	assert.Equal(t, expectedYaml, yamlText)
}

func TestCompareGetEnvVars(t *testing.T) {
	dataYaml := `unknown_setting: 123`

	t.Run("With BindEnv", func(t *testing.T) {
		viperConf, ntmConf := constructBothConfigs(dataYaml, false, func(cfg model.Setup) {
			cfg.BindEnv("port", "TEST_PORT")
			cfg.BindEnv("host", "TEST_HOST")
			cfg.BindEnv("log.level", "TEST_LOG_LEVEL")
		})

		viperEnvVars := viperConf.GetEnvVars()
		ntmEnvVars := ntmConf.GetEnvVars()

		sort.Strings(viperEnvVars)
		sort.Strings(ntmEnvVars)

		assert.Equal(t, viperEnvVars, ntmEnvVars, "viper and ntm should return the same environment variables")

		expected := []string{"TEST_HOST", "TEST_LOG_LEVEL", "TEST_PORT"}
		for i, ev := range expected {
			assert.Equal(t, viperEnvVars[i], ev, "viper missing expected env var: %s", ev)
			assert.Equal(t, ntmEnvVars[i], ev, "ntm missing expected env var: %s", ev)
		}
	})

	t.Run("Without BindEnv", func(t *testing.T) {
		viperConf, ntmConf := constructBothConfigs(dataYaml, false, nil)

		assert.Empty(t, viperConf.GetEnvVars(), "viper should return no env vars without BindEnv")
		assert.Empty(t, ntmConf.GetEnvVars(), "ntm should return no env vars without BindEnv")
	})

	t.Run("With EnvPrefix", func(t *testing.T) {
		viperConf, ntmConf := constructBothConfigs(dataYaml, false, func(cfg model.Setup) {
			cfg.SetEnvPrefix("MYAPP")
			cfg.BindEnv("port") // No explicit name — will use prefix
		})

		expected := "MYAPP_PORT"

		assert.Contains(t, viperConf.GetEnvVars(), expected, "viper should apply EnvPrefix")
		assert.Contains(t, ntmConf.GetEnvVars(), expected, "ntm should apply EnvPrefix")
	})

	t.Run("With EnvKeyReplacer", func(t *testing.T) {
		viperConf, ntmConf := constructBothConfigs(dataYaml, false, func(cfg model.Setup) {
			cfg.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
			cfg.BindEnv("log.level")
		})

		expected := "DD_LOG_LEVEL" // Default prefix is "DD" for viper and ntm when initializing the config

		assert.Contains(t, viperConf.GetEnvVars(), expected, "viper should apply EnvKeyReplacer")
		assert.Contains(t, ntmConf.GetEnvVars(), expected, "ntm should apply EnvKeyReplacer")
	})

	t.Run("With EnvPrefix and EnvKeyReplacer", func(t *testing.T) {
		viperConf, ntmConf := constructBothConfigs(dataYaml, false, func(cfg model.Setup) {
			cfg.SetEnvPrefix("MYAPP")
			cfg.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
			cfg.BindEnv("db.connection.url")
		})

		expected := "MYAPP_DB_CONNECTION_URL"

		assert.Contains(t, viperConf.GetEnvVars(), expected, "viper should apply prefix and replacer")
		assert.Contains(t, ntmConf.GetEnvVars(), expected, "ntm should apply prefix and replacer")
	})

	t.Run("Adding an unknown setting in the yaml", func(t *testing.T) {
		viperConf, ntmConf := constructBothConfigs(dataYaml, false, func(cfg model.Setup) {
			cfg.SetKnown("PORT")
			cfg.SetDefault("HOST", "localhost")
			cfg.BindEnv("log_level")
		})

		viperEnvVars := viperConf.GetEnvVars()
		ntmEnvVars := ntmConf.GetEnvVars()

		sort.Strings(viperEnvVars)
		sort.Strings(ntmEnvVars)

		expected := []string{"DD_LOG_LEVEL"}
		assert.Equal(t, viperEnvVars, expected, "viper should return only known env vars")
		assert.Equal(t, ntmEnvVars, expected, "ntm should return only known env vars")
	})
}

func TestCompareGetKnownKeysLowercased(t *testing.T) {
	t.Run("Includes SetKnown keys", func(t *testing.T) {
		viperConf, ntmConf := constructBothConfigs("", false, func(cfg model.Setup) {
			cfg.SetKnown("PORT")
			cfg.SetKnown("host")
		})

		wantKeys := []string{"port", "host"}
		assert.ElementsMatch(t, wantKeys, slices.Collect(maps.Keys(viperConf.GetKnownKeysLowercased())))
		assert.ElementsMatch(t, wantKeys, slices.Collect(maps.Keys(ntmConf.GetKnownKeysLowercased())))

	})

	t.Run("Includes defaults", func(t *testing.T) {
		viperConf, ntmConf := constructBothConfigs("", false, func(cfg model.Setup) {
			cfg.SetDefault("TIMEOUT", 30)
		})

		wantKeys := []string{"timeout"}
		assert.ElementsMatch(t, wantKeys, slices.Collect(maps.Keys(viperConf.GetKnownKeysLowercased())))
		assert.ElementsMatch(t, wantKeys, slices.Collect(maps.Keys(ntmConf.GetKnownKeysLowercased())))
	})

	t.Run("Includes environment bindings", func(t *testing.T) {
		t.Setenv("TEST_LOG_LEVEL", "debug")
		viperConf, ntmConf := constructBothConfigs("", false, func(cfg model.Setup) {
			cfg.BindEnv("log.level", "TEST_LOG_LEVEL")
		})

		wantKeys := []string{"log", "log.level"}
		assert.ElementsMatch(t, wantKeys, slices.Collect(maps.Keys(viperConf.GetKnownKeysLowercased())))
		assert.ElementsMatch(t, wantKeys, slices.Collect(maps.Keys(ntmConf.GetKnownKeysLowercased())))
	})

	t.Run("Combined known/default/env", func(t *testing.T) {
		t.Setenv("TEST_LOG_LEVEL", "debug")
		viperConf, ntmConf := constructBothConfigs("", false, func(cfg model.Setup) {
			cfg.SetKnown("PORT")
			cfg.SetDefault("TIMEOUT", 30)
			cfg.BindEnv("log.level", "TEST_LOG_LEVEL")
		})

		wantKeys := []string{"port", "timeout", "log", "log.level"}
		assert.ElementsMatch(t, wantKeys, slices.Collect(maps.Keys(viperConf.GetKnownKeysLowercased())))
		assert.ElementsMatch(t, wantKeys, slices.Collect(maps.Keys(ntmConf.GetKnownKeysLowercased())))
	})

	t.Run("Handles unknown YAML keys", func(t *testing.T) {
		yamlData := `
port: 8080
customKey1: 8080
customKey2: unused
`
		viperConf, ntmConf := constructBothConfigs(yamlData, true, func(cfg model.Setup) {
			cfg.SetKnown("port") // unrelated known key
		})

		// Expected to not include customKey if it’s unknown
		assert.ElementsMatch(t, []string{"port"}, slices.Collect(maps.Keys(viperConf.GetKnownKeysLowercased())))
		assert.ElementsMatch(t, []string{"port"}, slices.Collect(maps.Keys(ntmConf.GetKnownKeysLowercased())))
		assert.NotContains(t, slices.Collect(maps.Keys(viperConf.GetKnownKeysLowercased())), "customkey1")
		assert.NotContains(t, slices.Collect(maps.Keys(ntmConf.GetKnownKeysLowercased())), "customkey1")
		assert.NotContains(t, slices.Collect(maps.Keys(viperConf.GetKnownKeysLowercased())), "customkey2")
		assert.NotContains(t, slices.Collect(maps.Keys(ntmConf.GetKnownKeysLowercased())), "customkey2")
	})

	t.Run("SetWithoutSource won't create known key", func(t *testing.T) {
		dataYaml := `
port: 8080
`
		viperConf, ntmConf := constructBothConfigs(dataYaml, true, func(cfg model.Setup) {
			cfg.SetKnown("port")
		})
		viperConf.SetWithoutSource("unknown_key.unknown_subkey", "true")
		ntmConf.SetWithoutSource("unknown_key.unknown_subkey", "true")

		wantKeys := []string{"port"}
		assert.ElementsMatch(t, wantKeys, slices.Collect(maps.Keys(viperConf.GetKnownKeysLowercased())))
		assert.ElementsMatch(t, wantKeys, slices.Collect(maps.Keys(ntmConf.GetKnownKeysLowercased())))
	})

}

func TestCompareAllKeysLowercased(t *testing.T) {
	t.Run("Keys from YAML only", func(t *testing.T) {
		dataYaml := `
port: 8080
host: localhost
log:
  level: info
`
		viperConf, ntmConf := constructBothConfigs(dataYaml, true, func(cfg model.Setup) {
			cfg.SetKnown("port")
			cfg.SetKnown("host")
			cfg.SetKnown("log.level")
		})

		wantKeys := []string{"port", "host", "log.level"}
		assert.ElementsMatch(t, wantKeys, viperConf.AllKeysLowercased())
		assert.ElementsMatch(t, wantKeys, ntmConf.AllKeysLowercased())
	})

	t.Run("Keys from defaults", func(t *testing.T) {
		viperConf, ntmConf := constructBothConfigs("", false, func(cfg model.Setup) {
			cfg.SetDefault("PORT", 8080)
			cfg.SetDefault("HOST", "localhost")
			cfg.SetDefault("log.level", "info")
		})

		wantKeys := []string{"port", "host", "log.level"}
		assert.ElementsMatch(t, wantKeys, viperConf.AllKeysLowercased())
		assert.ElementsMatch(t, wantKeys, ntmConf.AllKeysLowercased())
	})

	t.Run("Keys from mixed sources", func(t *testing.T) {
		t.Setenv("TEST_API_KEY", "12345")

		dataYaml := `port: 8080`
		viperConf, ntmConf := constructBothConfigs(dataYaml, false, func(cfg model.Setup) {
			cfg.SetKnown("port")
			cfg.SetDefault("HOST", "localhost")
			cfg.BindEnv("api_key", "TEST_API_KEY")
		})

		wantKeys := []string{"port", "host", "api_key"}
		assert.ElementsMatch(t, wantKeys, viperConf.AllKeysLowercased())
		assert.ElementsMatch(t, wantKeys, ntmConf.AllKeysLowercased())
	})

	t.Run("Includes unknown YAML keys", func(t *testing.T) {
		dataYaml := `
port: 8080
host: 
customKey1:
customKey2: unused
`

		viperConf, ntmConf := constructBothConfigs(dataYaml, true, func(cfg model.Setup) {
			cfg.SetKnown("port")
			cfg.SetKnown("host")
		})

		wantKeys := []string{"port", "host", "customkey1", "customkey2"}
		assert.ElementsMatch(t, wantKeys, viperConf.AllKeysLowercased())
		assert.ElementsMatch(t, wantKeys, ntmConf.AllKeysLowercased())
	})

	t.Run("SetWithoutSource will create unknown key", func(t *testing.T) {
		dataYaml := `
port: 8080
`
		viperConf, ntmConf := constructBothConfigs(dataYaml, true, func(cfg model.Setup) {
			cfg.SetKnown("port")
		})
		viperConf.SetWithoutSource("unknown_key.unknown_subkey", "true")
		ntmConf.SetWithoutSource("unknown_key.unknown_subkey", "true")

		wantKeys := []string{"port", "unknown_key.unknown_subkey"}
		assert.ElementsMatch(t, wantKeys, viperConf.AllKeysLowercased())
		assert.ElementsMatch(t, wantKeys, ntmConf.AllKeysLowercased())
	})
}

func TestCompareIsConfigured(t *testing.T) {
	dataYaml := `
apm_config:
  telemetry:
    dd_url: https://example.com/
`
	t.Setenv("DD_RUNTIME_SECURITY_CONFIG_ENDPOINTS_DD_URL", "https://example.com/endpoint/")

	viperConf, ntmConf := constructBothConfigs(dataYaml, true, func(cfg model.Setup) {
		cfg.SetKnown("apm_config.telemetry.dd_url")
		cfg.SetKnown("database_monitoring.samples.dd_url")
		cfg.SetKnown("runtime_security_config.endpoints.dd_url")
		cfg.SetDefault("apm_config.telemetry.dd_url", "")
		cfg.SetDefault("database_monitoring.samples.dd_url", "")
		cfg.BindEnvAndSetDefault("runtime_security_config.endpoints.dd_url", "", "DD_RUNTIME_SECURITY_CONFIG_ENDPOINTS_DD_URL")
	})

	assert.True(t, viperConf.IsConfigured("apm_config.telemetry"))
	assert.True(t, ntmConf.IsConfigured("apm_config.telemetry"))

	assert.True(t, viperConf.IsConfigured("apm_config.telemetry.dd_url"))
	assert.True(t, ntmConf.IsConfigured("apm_config.telemetry.dd_url"))

	assert.False(t, viperConf.IsConfigured("database_monitoring.samples"))
	assert.False(t, ntmConf.IsConfigured("database_monitoring.samples"))

	assert.False(t, viperConf.IsConfigured("database_monitoring.samples.dd_url"))
	assert.False(t, ntmConf.IsConfigured("database_monitoring.samples.dd_url"))

	assert.True(t, viperConf.IsConfigured("runtime_security_config.endpoints"))
	assert.True(t, ntmConf.IsConfigured("runtime_security_config.endpoints"))

	assert.True(t, viperConf.IsConfigured("runtime_security_config.endpoints.dd_url"))
	assert.True(t, ntmConf.IsConfigured("runtime_security_config.endpoints.dd_url"))

}

func TestCompareConflictDataType(t *testing.T) {
	var yamlPayload = `
a: orange
c: 1234
`
	viperConf, ntmConf := constructBothConfigs(yamlPayload, true, func(cfg model.Setup) {
		cfg.SetDefault("a", "apple")
		cfg.SetDefault("c.d", true)
	})

	cvalue := viperConf.Get("c")
	assert.Equal(t, cvalue, 1234)

	dvalue := viperConf.Get("c.d")
	assert.Equal(t, dvalue, nil)

	// NOTE: Behavior difference, but it requires an error in the config
	cvalue = ntmConf.Get("c")
	assert.Equal(t, cvalue, map[string]interface{}{"d": true})

	dvalue = ntmConf.Get("c.d")
	assert.Equal(t, dvalue, true)
}
