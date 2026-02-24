// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package nodetreemodel defines a model for the config using a tree of nodes
package nodetreemodel

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v2"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/helper"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/viperconfig"
)

func constructBothConfigs(content string, dynamicSchema bool, setupFunc func(model.Setup)) (model.BuildableConfig, model.BuildableConfig) {
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
	} else {
		viperConf.ReadInConfig()
		ntmConf.ReadInConfig()
	}

	return viperConf, ntmConf
}

func TestCompareGetInt(t *testing.T) {
	dataYaml := `port: 345`
	viperConf, ntmConf := constructBothConfigs(dataYaml, true, nil)

	assert.Equal(t, 345, viperConf.GetInt("port"))
	assert.Equal(t, 345, ntmConf.GetInt("port"))

	viperConf, ntmConf = constructBothConfigs(dataYaml, false, func(cfg model.Setup) {
		cfg.SetKnown("port") //nolint:forbidigo // testing behavior
	})
	assert.Equal(t, 345, viperConf.GetInt("port"))
	assert.Equal(t, 345, ntmConf.GetInt("port"))
}

func TestCompareGetTypesLikeDefault(t *testing.T) {
	t.Setenv("DD_MY_FEATURE_ENABLED", "true")
	t.Setenv("DD_PORT", "345")
	viperConf, ntmConf := constructBothConfigs("", false, func(cfg model.Setup) {
		cfg.BindEnvAndSetDefault("my_feature.enabled", false)
		cfg.BindEnvAndSetDefault("port", 0)
	})

	assert.Equal(t, true, viperConf.Get("my_feature.enabled"))
	assert.Equal(t, 345, viperConf.Get("port"))

	assert.Equal(t, true, ntmConf.Get("my_feature.enabled"))
	assert.Equal(t, 345, ntmConf.Get("port"))
}

func TestCompareIsSet(t *testing.T) {
	dataYaml := `port: 345`
	viperConf, ntmConf := constructBothConfigs(dataYaml, true, nil)
	assert.Equal(t, true, viperConf.IsSet("port"))
	assert.Equal(t, true, ntmConf.IsSet("port"))

	viperConf, ntmConf = constructBothConfigs(dataYaml, false, func(cfg model.Setup) {
		cfg.SetKnown("port") //nolint:forbidigo // testing behavior
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
		cfg.BindEnv("port", "TEST_PORT") //nolint:forbidigo // testing behavior
	})
	assert.Equal(t, 789, viperConf.GetInt("port"))
	assert.Equal(t, 789, ntmConf.GetInt("port"))
	assert.Equal(t, true, viperConf.IsSet("port"))
	assert.Equal(t, true, ntmConf.IsSet("port"))

	t.Setenv("TEST_PORT", "")
	viperConf, ntmConf = constructBothConfigs(dataYaml, false, func(cfg model.Setup) {
		cfg.BindEnv("port", "TEST_PORT") //nolint:forbidigo // testing behavior
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

func TestCompareAllFlattenedSettingsWithSequenceID(t *testing.T) {
	t.Setenv("DD_MY_FEATURE_ENABLED", "true")
	t.Setenv("DD_PORT", "345")
	viperConf, ntmConf := constructBothConfigs("", false, func(cfg model.Setup) {
		cfg.BindEnvAndSetDefault("my_feature.enabled", false)
		cfg.BindEnvAndSetDefault("port", 0)
	})

	vipermap, _ := viperConf.AllFlattenedSettingsWithSequenceID()
	ntmmap, _ := ntmConf.AllFlattenedSettingsWithSequenceID()

	expectmap := map[string]interface{}{
		"my_feature.enabled": true,
		"port":               345,
	}
	assert.Equal(t, expectmap, vipermap)
	assert.Equal(t, expectmap, ntmmap)
}

func TestCompareGetEnvVars(t *testing.T) {
	dataYaml := `unknown_setting: 123`

	t.Run("With BindEnv", func(t *testing.T) {
		viperConf, ntmConf := constructBothConfigs(dataYaml, false, func(cfg model.Setup) {
			cfg.BindEnv("port", "TEST_PORT")           //nolint:forbidigo // testing behavior
			cfg.BindEnv("host", "TEST_HOST")           //nolint:forbidigo // testing behavior
			cfg.BindEnv("log.level", "TEST_LOG_LEVEL") //nolint:forbidigo // testing behavior
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
			cfg.BindEnv("port") //nolint:forbidigo // testing behavior // No explicit name — will use prefix
		})

		expected := "MYAPP_PORT"

		assert.Contains(t, viperConf.GetEnvVars(), expected, "viper should apply EnvPrefix")
		assert.Contains(t, ntmConf.GetEnvVars(), expected, "ntm should apply EnvPrefix")
	})

	t.Run("With EnvKeyReplacer", func(t *testing.T) {
		viperConf, ntmConf := constructBothConfigs(dataYaml, false, func(cfg model.Setup) {
			cfg.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
			cfg.BindEnv("log.level") //nolint:forbidigo // testing behavior
		})

		expected := "DD_LOG_LEVEL" // Default prefix is "DD" for viper and ntm when initializing the config

		assert.Contains(t, viperConf.GetEnvVars(), expected, "viper should apply EnvKeyReplacer")
		assert.Contains(t, ntmConf.GetEnvVars(), expected, "ntm should apply EnvKeyReplacer")
	})

	t.Run("With EnvPrefix and EnvKeyReplacer", func(t *testing.T) {
		viperConf, ntmConf := constructBothConfigs(dataYaml, false, func(cfg model.Setup) {
			cfg.SetEnvPrefix("MYAPP")
			cfg.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
			cfg.BindEnv("db.connection.url") //nolint:forbidigo // testing behavior
		})

		expected := "MYAPP_DB_CONNECTION_URL"

		assert.Contains(t, viperConf.GetEnvVars(), expected, "viper should apply prefix and replacer")
		assert.Contains(t, ntmConf.GetEnvVars(), expected, "ntm should apply prefix and replacer")
	})

	t.Run("Adding an unknown setting in the yaml", func(t *testing.T) {
		viperConf, ntmConf := constructBothConfigs(dataYaml, false, func(cfg model.Setup) {
			cfg.SetKnown("PORT") //nolint:forbidigo // testing behavior
			cfg.SetDefault("HOST", "localhost")
			cfg.BindEnv("log_level") //nolint:forbidigo // testing behavior
		})

		viperEnvVars := viperConf.GetEnvVars()
		ntmEnvVars := ntmConf.GetEnvVars()

		sort.Strings(viperEnvVars)
		sort.Strings(ntmEnvVars)

		expected := []string{"DD_LOG_LEVEL"}
		assert.Equal(t, viperEnvVars, expected, "viper should return only known env vars")
		assert.Equal(t, ntmEnvVars, expected, "ntm should return only known env vars")
	})

	t.Run("Duplicate env vars", func(t *testing.T) {
		viperConf, ntmConf := constructBothConfigs(dataYaml, false, func(cfg model.Setup) {
			cfg.BindEnv("test", "ABC")  //nolint:forbidigo // testing behavior
			cfg.BindEnv("test2", "ABC") //nolint:forbidigo // testing behavior
			cfg.BindEnv("test3")        //nolint:forbidigo // testing behavior
		})

		viperEnvVars := viperConf.GetEnvVars()
		ntmEnvVars := ntmConf.GetEnvVars()

		sort.Strings(viperEnvVars)
		sort.Strings(ntmEnvVars)

		expected := []string{"ABC", "DD_TEST3"}
		assert.Equal(t, viperEnvVars, expected, "viper should return only known env vars")
		assert.Equal(t, ntmEnvVars, expected, "ntm should return only known env vars")
	})
}

func TestCompareAllSettings(t *testing.T) {
	t.Setenv("TEST_API_KEY", "12345")

	dataYaml := `timeout: 60`
	viperConf, ntmConf := constructBothConfigs(dataYaml, false, func(cfg model.Setup) {
		cfg.SetDefault("timeout", 30)
		cfg.SetDefault("host", "localhost")
		cfg.SetKnown("port")                   //nolint:forbidigo // testing behavior
		cfg.BindEnv("api_key", "TEST_API_KEY") //nolint:forbidigo // testing behavior
		cfg.BindEnv("log_level")               //nolint:forbidigo // testing behavior
		cfg.BindEnvAndSetDefault("logs_config.enabled", false)
	})

	// AllSettings does not include 'known' nor 'bindenv (undefined)'
	expect := map[string]interface{}{
		"timeout": 60,          // file
		"host":    "localhost", // default
		"api_key": "12345",     // env var (defined)
		"logs_config": map[string]interface{}{
			"enabled": false,
		},
	}
	assert.Equal(t, expect, viperConf.AllSettings())
	assert.Equal(t, expect, ntmConf.AllSettings())
}

func TestCompareGetKnownKeysLowercased(t *testing.T) {
	t.Run("Includes SetKnown keys", func(t *testing.T) {
		viperConf, ntmConf := constructBothConfigs("", false, func(cfg model.Setup) {
			cfg.SetKnown("PORT") //nolint:forbidigo // testing behavior
			cfg.SetKnown("host") //nolint:forbidigo // testing behavior
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
			cfg.BindEnv("log.level", "TEST_LOG_LEVEL") //nolint:forbidigo // testing behavior
		})

		wantKeys := []string{"log", "log.level"}
		assert.ElementsMatch(t, wantKeys, slices.Collect(maps.Keys(viperConf.GetKnownKeysLowercased())))
		assert.ElementsMatch(t, wantKeys, slices.Collect(maps.Keys(ntmConf.GetKnownKeysLowercased())))
	})

	t.Run("Includes env var even if undefined", func(t *testing.T) {
		viperConf, ntmConf := constructBothConfigs("", false, func(cfg model.Setup) {
			cfg.BindEnv("log.level", "TEST_LOG_LEVEL") //nolint:forbidigo // testing behavior
		})

		wantKeys := []string{"log", "log.level"}
		assert.ElementsMatch(t, wantKeys, slices.Collect(maps.Keys(viperConf.GetKnownKeysLowercased())))
		assert.ElementsMatch(t, wantKeys, slices.Collect(maps.Keys(ntmConf.GetKnownKeysLowercased())))
	})

	t.Run("Combined known/default/env", func(t *testing.T) {
		t.Setenv("TEST_LOG_LEVEL", "debug")
		viperConf, ntmConf := constructBothConfigs("", false, func(cfg model.Setup) {
			cfg.SetKnown("PORT") //nolint:forbidigo // testing behavior
			cfg.SetDefault("TIMEOUT", 30)
			cfg.BindEnv("log.level", "TEST_LOG_LEVEL") //nolint:forbidigo // testing behavior
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
			cfg.SetKnown("port") //nolint:forbidigo // testing behavior // unrelated known key
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
			cfg.SetKnown("port") //nolint:forbidigo // testing behavior
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
			cfg.SetKnown("port")      //nolint:forbidigo // testing behavior
			cfg.SetKnown("host")      //nolint:forbidigo // testing behavior
			cfg.SetKnown("log.level") //nolint:forbidigo // testing behavior
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
			cfg.SetKnown("port") //nolint:forbidigo // testing behavior
			cfg.SetDefault("HOST", "localhost")
			cfg.BindEnv("api_key", "TEST_API_KEY") //nolint:forbidigo // testing behavior
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
			cfg.SetKnown("port") //nolint:forbidigo // testing behavior
			cfg.SetKnown("host") //nolint:forbidigo // testing behavior
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
			cfg.SetKnown("port") //nolint:forbidigo // testing behavior
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
		cfg.SetKnown("apm_config.telemetry.dd_url")              //nolint:forbidigo // testing behavior
		cfg.SetKnown("database_monitoring.samples.dd_url")       //nolint:forbidigo // testing behavior
		cfg.SetKnown("runtime_security_config.endpoints.dd_url") //nolint:forbidigo // testing behavior
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

func TestCompareEmptyConfigSection(t *testing.T) {
	// Create a config yaml file that only declares sections but no individual settings
	dataYaml := `
apm_config:
  telemetry:
database_monitoring:
  samples:
logs_config:
  auto_multi_line:
runtime_security_config:
  endpoints:
unknown_section:
  info:
`
	// apm_config.telemetry        - declared "known" (silences warnings, not added to schema, not very useful to do)
	// database_monitoring.samples - defines a default
	// logs_config.auto_multi_line - bind an env var and assign a value to that env var
	// runtime_security_config.endpoints - bind an env var but leave that env var undefined
	// unknown_section.info        - undefined, neither default nor env var, only shows up in the config.yaml
	// additional_endpoints        - defines a default, does not appear in the file

	t.Setenv("DD_LOGS_CONFIG_AUTO_MULTI_LINE_TOKENIZER_MAX_INPUT_BYTES", "100")

	viperConf, ntmConf := constructBothConfigs(dataYaml, true, func(cfg model.Setup) {
		cfg.SetKnown("apm_config.telemetry.dd_url") //nolint:forbidigo // test behavior for compatibility
		cfg.SetDefault("database_monitoring.samples.dd_url", "")
		cfg.BindEnv("runtime_security_config.endpoints.dd_url", "DD_RUNTIME_SECURITY_CONFIG_ENDPOINTS_DD_URL")                           //nolint:forbidigo // test behavior for compatibility
		cfg.BindEnv("logs_config.auto_multi_line.tokenizer_max_input_bytes", "DD_LOGS_CONFIG_AUTO_MULTI_LINE_TOKENIZER_MAX_INPUT_BYTES") //nolint:forbidigo // test behavior for compatibility
		cfg.BindEnvAndSetDefault("additional_endpoints", map[string][]string{})
	})

	// NOTE: AllKeysLowercased does not match between the implementations.
	expectedKeys := []string{
		"additional_endpoints",
		"apm_config.telemetry",
		//"apm_config.telemetry.dd_url", (missing)
		"database_monitoring.samples",
		//"database_monitoring.samples.dd_url", (missing)
		"logs_config.auto_multi_line",
		"logs_config.auto_multi_line.tokenizer_max_input_bytes",
		"runtime_security_config.endpoints",
		"runtime_security_config.endpoints.dd_url",
		"unknown_section.info",
	}
	expectedKeys2 := []string{
		"additional_endpoints",
		"apm_config.telemetry",
		"apm_config.telemetry.dd_url",
		//"database_monitoring.samples", (missing)
		"database_monitoring.samples.dd_url",
		"logs_config.auto_multi_line",
		"logs_config.auto_multi_line.tokenizer_max_input_bytes",
		"runtime_security_config.endpoints",
		"runtime_security_config.endpoints.dd_url",
		"unknown_section.info",
	}
	assert.Equal(t, expectedKeys, viperConf.AllKeysLowercased())
	assert.Equal(t, expectedKeys2, ntmConf.AllKeysLowercased())

	// AllSettings does not match either.
	// - viper doesn't split "auto_multi_line.tokenizer_max_input_bytes" because it comes from an env var
	expectedSettings := map[string]interface{}{
		"additional_endpoints": map[string][]string{},
		"database_monitoring": map[string]interface{}{
			"samples": map[string]interface{}{
				"dd_url": "",
			},
		},
		"logs_config": map[string]interface{}{
			"auto_multi_line.tokenizer_max_input_bytes": "100",
		},
	}
	expectedSettings2 := map[string]interface{}{
		"additional_endpoints": map[string][]string{},
		"apm_config": map[string]interface{}{
			"telemetry": nil,
		},
		"runtime_security_config": map[string]interface{}{
			"endpoints": nil,
		},
		"database_monitoring": map[string]interface{}{
			"samples": map[string]interface{}{
				"dd_url": "",
			},
		},
		"logs_config": map[string]interface{}{
			"auto_multi_line": map[string]interface{}{
				"tokenizer_max_input_bytes": "100",
			},
		},
		"unknown_section": map[string]interface{}{
			"info": nil,
		},
	}
	assert.Equal(t, expectedSettings, viperConf.AllSettings())
	assert.Equal(t, expectedSettings2, ntmConf.AllSettings())

	////////////
	// tests for IsConfigured

	// not configured because known does not define a setting
	assert.False(t, viperConf.IsConfigured("apm_config.telemetry"))
	assert.False(t, ntmConf.IsConfigured("apm_config.telemetry"))

	// not configured by the file nor env var
	assert.False(t, viperConf.IsConfigured("apm_config.telemetry.enabled"))
	assert.False(t, ntmConf.IsConfigured("apm_config.telemetry.enabled"))

	// not configured, because only default is defined
	assert.False(t, viperConf.IsConfigured("database_monitoring.samples"))
	assert.False(t, ntmConf.IsConfigured("database_monitoring.samples"))

	// not configured, an env var is bound but that env var is undefined
	assert.False(t, viperConf.IsConfigured("runtime_security_config.endpoints"))
	assert.False(t, ntmConf.IsConfigured("runtime_security_config.endpoints"))

	// yes configured, because an env var is defined that contains this setting
	assert.True(t, viperConf.IsConfigured("logs_config.auto_multi_line"))
	assert.True(t, ntmConf.IsConfigured("logs_config.auto_multi_line"))

	// not configured, unknown section
	assert.False(t, viperConf.IsConfigured("unknown_section.info"))
	assert.False(t, ntmConf.IsConfigured("unknown_section.info"))

	// not configured, because only default is defined
	assert.False(t, viperConf.IsConfigured("additional_endpoints"))
	assert.False(t, ntmConf.IsConfigured("aditional_endpoints"))

	////////////
	// tests for IsSet

	// False, apm_config.telemetry.dd_url is known, but that does not define it in
	// the schema. This node is not set.
	// IsSet gives inconsistent results
	assert.False(t, viperConf.IsSet("apm_config.telemetry"))
	assert.True(t, ntmConf.IsSet("apm_config.telemetry"))

	// not set because this setting is not defined
	assert.False(t, viperConf.IsSet("apm_config.telemetry.enabled"))
	assert.False(t, ntmConf.IsSet("apm_config.telemetry.enabled"))

	// this has a default value so it IsSet
	assert.True(t, viperConf.IsSet("database_monitoring.samples"))
	assert.True(t, ntmConf.IsSet("database_monitoring.samples"))

	// IsSet gives inconsistent results
	assert.False(t, viperConf.IsSet("runtime_security_config.endpoints"))
	assert.True(t, ntmConf.IsSet("runtime_security_config.endpoints"))

	// Viper arguably should return true here, but Viper doesn't
	// connect bound env vars to their parent settings. This bug in viper
	// is a good reason to encourage callers to switch to IsConfigured
	assert.False(t, viperConf.IsSet("logs_config.auto_multi_line"))
	assert.True(t, ntmConf.IsSet("logs_config.auto_multi_line"))

	// Unclear what the correct behavior here should be since this setting is unknown.
	assert.False(t, viperConf.IsSet("unknown_section.info"))
	assert.True(t, ntmConf.IsSet("unknown_section.info"))

	// IsSet gives inconsistent results
	assert.True(t, viperConf.IsSet("additional_endpoints"))
	assert.False(t, ntmConf.IsSet("aditional_endpoints"))

	////////////
	// tests for HasSection

	// HasSection true for an empty section
	assert.True(t, viperConf.HasSection("apm_config.telemetry"))
	assert.True(t, ntmConf.HasSection("apm_config.telemetry"))

	// False because this setting isn't defined at all
	assert.False(t, viperConf.HasSection("apm_config.telemetry.enabled"))
	assert.False(t, ntmConf.HasSection("apm_config.telemetry.enabled"))

	// HasSection true for an empty section
	assert.True(t, viperConf.HasSection("database_monitoring.samples"))
	assert.True(t, ntmConf.HasSection("database_monitoring.samples"))

	// HasSection true for an empty section
	assert.True(t, viperConf.HasSection("runtime_security_config.endpoints"))
	assert.True(t, ntmConf.HasSection("runtime_security_config.endpoints"))

	// HasSection true because the section has data
	assert.True(t, viperConf.HasSection("logs_config.auto_multi_line"))
	assert.True(t, ntmConf.HasSection("logs_config.auto_multi_line"))

	// HasSection true for an empty section, even though it is unknown
	assert.True(t, viperConf.HasSection("unknown_section.info"))
	assert.True(t, ntmConf.HasSection("unknown_section.info"))

	// False because this is not defined (aside from default)
	assert.False(t, viperConf.HasSection("additional_endpoints"))
	assert.False(t, ntmConf.HasSection("aditional_endpoints"))
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
	assert.Equal(t, 1234, cvalue)

	dvalue := viperConf.Get("c.d")
	assert.Equal(t, nil, dvalue)

	cvalue = ntmConf.Get("c")
	assert.Equal(t, 1234, cvalue)

	dvalue = ntmConf.Get("c.d")
	assert.Equal(t, nil, dvalue)
}

func TestCompareTimeDuration(t *testing.T) {
	viperConf, ntmConf := constructBothConfigs("", false, func(cfg model.Setup) {
		cfg.SetDefault("provider.interval", 5*time.Second)
		cfg.SetDefault("lookup_timeout", 30*time.Millisecond)
	})
	assert.Equal(t, 5*time.Second, viperConf.GetDuration("provider.interval"))
	assert.Equal(t, 5*time.Second, ntmConf.GetDuration("provider.interval"))

	assert.Equal(t, 30*time.Millisecond, viperConf.GetDuration("lookup_timeout"))
	assert.Equal(t, 30*time.Millisecond, ntmConf.GetDuration("lookup_timeout"))

	// refuse to convert time.Duration to int64
	assert.Equal(t, int64(0), viperConf.GetInt64("lookup_timeout"))
	assert.Equal(t, int64(0), ntmConf.GetInt64("lookup_timeout"))

	assert.Equal(t, 30*time.Millisecond, viperConf.Get("lookup_timeout"))
	assert.Equal(t, 30*time.Millisecond, ntmConf.Get("lookup_timeout"))
}

func TestReadConfigReset(t *testing.T) {
	initialYAML := `port: 1234`
	overrideYAML := `host: localhost`

	viperConf, ntmConf := constructBothConfigs(initialYAML, true, func(cfg model.Setup) {
		cfg.SetKnown("port") //nolint:forbidigo // testing behavior
		cfg.SetKnown("host") //nolint:forbidigo // testing behavior
	})

	assert.Equal(t, 1234, viperConf.GetInt("port"))
	assert.Equal(t, 1234, ntmConf.GetInt("port"))
	assert.False(t, viperConf.IsSet("host"))
	assert.False(t, ntmConf.IsSet("host"))

	// Now use ReadConfig to reset with only "host"
	viperConf.SetConfigType("yaml")
	err := viperConf.ReadConfig(bytes.NewBuffer([]byte(overrideYAML)))
	assert.NoError(t, err)

	ntmConf.SetConfigType("yaml")
	err = ntmConf.ReadConfig(bytes.NewBuffer([]byte(overrideYAML)))
	assert.NoError(t, err)

	// After ReadConfig, "port" should be gone, "host" should be set
	assert.False(t, viperConf.IsSet("port"), "viper should have cleared previous config")
	assert.False(t, ntmConf.IsSet("port"), "ntm should have cleared previous config")

	assert.Equal(t, "localhost", viperConf.GetString("host"))
	assert.Equal(t, "localhost", ntmConf.GetString("host"))
}

func TestReadInConfigResetsPreviousConfig(t *testing.T) {
	tempDir := t.TempDir()

	configA := `port: 8123`
	configAPath := filepath.Join(tempDir, "configA.yaml")
	err := os.WriteFile(configAPath, []byte(configA), 0o644)
	assert.NoError(t, err)

	configB := `host: localhost`
	configBPath := filepath.Join(tempDir, "configB.yaml")
	err = os.WriteFile(configBPath, []byte(configB), 0o644)
	assert.NoError(t, err)

	// Set up Viper and NTM with configA loaded via ReadInConfig
	viperConf := viperconfig.NewViperConfig("datadog", "DD", strings.NewReplacer(".", "_"))
	ntmConf := NewNodeTreeConfig("datadog", "DD", strings.NewReplacer(".", "_"))

	viperConf.SetTestOnlyDynamicSchema(true)
	ntmConf.SetTestOnlyDynamicSchema(true)
	viperConf.SetConfigFile(configAPath)
	ntmConf.SetConfigFile(configAPath)

	viperConf.BuildSchema()
	ntmConf.BuildSchema()

	err = viperConf.ReadInConfig()
	assert.NoError(t, err)
	err = ntmConf.ReadInConfig()
	assert.NoError(t, err)

	assert.Equal(t, 8123, viperConf.GetInt("port"))
	assert.Equal(t, 8123, ntmConf.GetInt("port"))
	assert.False(t, viperConf.IsSet("host"))
	assert.False(t, ntmConf.IsSet("host"))

	// Update config file to configB (overwrites configA)
	viperConf.SetConfigFile(configBPath)
	ntmConf.SetConfigFile(configBPath)

	err = viperConf.ReadInConfig()
	assert.NoError(t, err)
	err = ntmConf.ReadInConfig()
	assert.NoError(t, err)

	assert.False(t, viperConf.IsSet("port"), "viper should have cleared previous config")
	assert.False(t, ntmConf.IsSet("port"), "ntm should have cleared previous config")
	// "host" should now be available
	assert.Equal(t, "localhost", viperConf.GetString("host"))
	assert.Equal(t, "localhost", ntmConf.GetString("host"))
}

func TestReadInConfigExactError(t *testing.T) {
	// Invalid YAML that will fail to parse
	dataYaml := `site:datadoghq.eu
`
	viperConf, ntmConf := constructBothConfigs(dataYaml, false, func(cfg model.Setup) {
		cfg.BindEnvAndSetDefault("site", "datadoghq.com")
	})

	// Both config implementations should return "Config File Not Found" when
	// a parsing error is encountered
	err := viperConf.ReadInConfig()
	assert.ErrorIs(t, err, model.ErrConfigFileNotFound)
	err = ntmConf.ReadInConfig()
	assert.ErrorIs(t, err, model.ErrConfigFileNotFound)

	assert.Equal(t, "datadoghq.com", viperConf.GetString("site"))
	assert.Equal(t, "datadoghq.com", ntmConf.GetString("site"))
}

func TestCompareEnvVarsSubfields(t *testing.T) {
	t.Run("Subsettings are merged with env vars", func(t *testing.T) {
		data, _ := json.Marshal(map[string]string{"a": "apple"})
		t.Setenv("TEST_MY_FEATURE_INFO_TARGETS", string(data))

		configData := `
my_feature:
  info:
    enabled: true
`
		viperConf, ntmConf := constructBothConfigs(configData, false, func(cfg model.Setup) {
			cfg.BindEnvAndSetDefault("my_feature.info.name", "feat")
			cfg.BindEnvAndSetDefault("my_feature.info.enabled", false)
			cfg.BindEnvAndSetDefault("my_feature.info.version", "v2")
			cfg.BindEnv("my_feature.info.targets", "TEST_MY_FEATURE_INFO_TARGETS") //nolint: forbidigo // testing behavior
		})

		fields := viperConf.GetSubfields("my_feature.info")
		assert.Equal(t, []string{"enabled", "name", "targets", "version"}, fields)

		fields = ntmConf.GetSubfields("my_feature.info")
		assert.Equal(t, []string{"enabled", "name", "targets", "version"}, fields)
	})
}

func TestInvalidFileData(t *testing.T) {
	configData := `
fruit:
  apple:
  banana:
  cherry:
  donut:
    dozen: 12
`

	t.Setenv("DD_FRUIT_CHERRY_SEED_NUM", "5")
	viperConf, ntmConf := constructBothConfigs(configData, false, func(cfg model.Setup) {
		// default wins over invalid file
		cfg.BindEnvAndSetDefault("fruit.apple.core.seeds", 2)

		// file only (missing default)
		cfg.BindEnv("fruit.banana.peel.color") //nolint:forbidigo // legit usage, testing compatibility with viper

		// env wins over file
		cfg.BindEnv("fruit.cherry.seed.num") //nolint:forbidigo // legit usage, testing compatibility with viper
	})

	expectAppleMap := map[string]interface{}{
		"core": map[string]interface{}{
			"seeds": 2,
		},
	}

	assert.Equal(t, expectAppleMap, viperConf.Get("fruit.apple"))
	assert.Equal(t, nil, viperConf.Get("fruit.banana"))
	assert.Equal(t, 2, viperConf.GetInt("fruit.apple.core.seeds"))
	assert.Equal(t, "", viperConf.GetString("fruit.banana.peel.color"))
	assert.Equal(t, 5, viperConf.GetInt("fruit.cherry.seed.num"))
	assert.Equal(t, 12, viperConf.GetInt("fruit.donut.dozen"))

	assert.Equal(t, expectAppleMap, ntmConf.Get("fruit.apple"))
	assert.Equal(t, nil, ntmConf.Get("fruit.banana"))
	assert.Equal(t, 2, ntmConf.GetInt("fruit.apple.core.seeds"))
	assert.Equal(t, "", ntmConf.GetString("fruit.banana.peel.color"))
	assert.Equal(t, 5, ntmConf.GetInt("fruit.cherry.seed.num"))
	assert.Equal(t, 12, ntmConf.GetInt("fruit.donut.dozen"))
}

func TestCompareConfigUsesDotSeparatedFields(t *testing.T) {
	configData := `
my_feature.info.enabled: true
second_feature:
  info.enabled: true
additional_endpoints:
  https://url1.com:
    - my_api_key
`
	viperConf, ntmConf := constructBothConfigs(configData, false, func(cfg model.Setup) {
		cfg.BindEnvAndSetDefault("my_feature.info.name", "feat")
		cfg.BindEnvAndSetDefault("my_feature.info.enabled", false)
		cfg.BindEnvAndSetDefault("my_feature.info.version", "v2")
		cfg.BindEnvAndSetDefault("second_feature.info.enabled", false)
		cfg.BindEnvAndSetDefault("additional_endpoints", map[string][]string{})
	})

	assert.Equal(t, true, viperConf.Get("my_feature.info.enabled"))
	assert.Equal(t, true, ntmConf.Get("my_feature.info.enabled"))

	assert.Equal(t, true, viperConf.Get("second_feature.info.enabled"))
	assert.Equal(t, true, ntmConf.Get("second_feature.info.enabled"))

	expectEndpoints := map[string][]string{
		"https://url1.com": {"my_api_key"},
	}
	assert.Equal(t, expectEndpoints, viperConf.GetStringMapStringSlice("additional_endpoints"))
	assert.Equal(t, expectEndpoints, ntmConf.GetStringMapStringSlice("additional_endpoints"))
}

func TestGetViperCombineInvalidFileData(t *testing.T) {
	// The setting in the yaml file has the wrong shape
	// It is a list of an object, but it is supposed to not be a list
	// The implementation should handle this predictabily and compatibly with Viper
	// The rule when merging conflicts is that higher layers have branches kept, so
	// the invalid file data will be kept rather than the default values.
	configData := `network_path:
  collector:
    - input_chan_size: 23456
`
	// Two settings at path, but the file source has the wrong shape
	viperConf, ntmConf := constructBothConfigs(configData, false, func(cfg model.Setup) {
		cfg.BindEnvAndSetDefault("network_path.collector.input_chan_size", 0) //nolint:forbidigo // used to test behavior
		cfg.BindEnvAndSetDefault("network_path.collector.workers", 0)         //nolint:forbidigo // used to test behavior
	})

	// Value at the path is a list of map
	expectCollector := []interface{}{
		map[interface{}]interface{}{
			"input_chan_size": 23456,
		},
	}
	// Parent of that element
	expectNetworkPath := map[string]interface{}{
		"collector": []interface{}{
			map[interface{}]interface{}{
				"input_chan_size": 23456,
			},
		},
	}

	// Test what the methods `Get, GetViperCombine, AllSettings` return for each impl
	for _, cfg := range []model.Reader{viperConf, ntmConf} {
		assert.Equal(t, expectCollector, cfg.Get("network_path.collector"))
		assert.Equal(t, expectCollector, helper.GetViperCombine(cfg, "network_path.collector"))
		assert.Equal(t, expectCollector, cfg.AllSettings()["network_path"].(map[string]interface{})["collector"])

		// Test parent element as well
		actual := helper.GetViperCombine(cfg, "network_path")
		assert.Equal(t, expectNetworkPath, actual)
	}
}

func TestUnsetForSourceWithFile(t *testing.T) {
	yamlExample := []byte(`
some:
  setting: file_value
`)

	tempfile, err := os.CreateTemp("", "test-*.yaml")
	require.NoError(t, err, "failed to create temporary file")
	defer os.Remove(tempfile.Name())

	tempfile.Write(yamlExample)

	viperConf, ntmConf := constructBothConfigs("", false, func(cfg model.Setup) {
		cfg.SetDefault("some.setting", "default_value")
		cfg.SetConfigFile(tempfile.Name())
	})

	for name, cfg := range map[string]model.BuildableConfig{"ntm": ntmConf, "viper": viperConf} {
		t.Run(name, func(t *testing.T) {
			fmt.Printf("%v\n", cfg.GetAllSources("some.setting"))
			cfg.Set("some.setting", "runtime_value", model.SourceAgentRuntime)
			cfg.Set("some.setting", "process_value", model.SourceLocalConfigProcess)
			cfg.Set("some.setting", "RC_value", model.SourceRC)

			assert.Equal(t, "RC_value", cfg.GetString("some.setting"))

			cfg.UnsetForSource("some.setting", model.SourceRC)
			assert.Equal(t, "process_value", cfg.GetString("some.setting"))

			cfg.UnsetForSource("some.setting", model.SourceLocalConfigProcess)
			assert.Equal(t, "runtime_value", cfg.GetString("some.setting"))

			cfg.UnsetForSource("some.setting", model.SourceAgentRuntime)
			assert.Equal(t, "file_value", cfg.GetString("some.setting"))

			cfg.UnsetForSource("some.setting", model.SourceFile)
			assert.Equal(t, "default_value", cfg.GetString("some.setting"))

			cfg.UnsetForSource("some.setting", model.SourceDefault)
			assert.Equal(t, "", cfg.GetString("some.setting"))
		})
	}
}
