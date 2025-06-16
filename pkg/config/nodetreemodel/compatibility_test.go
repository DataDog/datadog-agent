// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package nodetreemodel defines a model for the config using a tree of nodes
package nodetreemodel

import (
	"bytes"
	"fmt"
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

	fmt.Printf("%s\n", ntmConf.Stringify("root"))

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

func TestCompareEmptyConfigSection(t *testing.T) {
	// Create a config yaml file that only declares "sections" but no individual settings
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
	// Due to how the yaml parser works, in memory this will actually look like this:
	//
	// apm_config:
	//   telemetry: ""
	//
	// etc
	//
	// In other words, "telemetry:" does not open up an "empty section", instead it is a leaf
	// whose value is nil / the empty string. This almost certainly does not match the user's
	// intent, whatever it may be. How this ends up being treated by the config is dependent
	// on the implementation.
	//
	// The rest of this test sees what happens in a different case for each of these sections:
	//
	// apm_config.telemetry        - declared "known" by the schema (generally, not a useful thing to do)
	// database_monitoring.samples - defines a default, the standard we want to end up at for each setting
	// logs_config.auto_multi_line - bind an env var and assign a value to that env var
	// runtime_security_config.endpoints - bind an env var but leave that env var undefined
	// unknown_section.info        - undefined, neither default nor env var, only shows up in the config.yaml

	t.Setenv("DD_LOGS_CONFIG_AUTO_MULTI_LINE_TOKENIZER_MAX_INPUT_BYTES", "100")

	viperConf, ntmConf := constructBothConfigs(dataYaml, true, func(cfg model.Setup) {
		cfg.SetKnown("apm_config.telemetry.dd_url")
		cfg.SetDefault("database_monitoring.samples.dd_url", "")
		cfg.BindEnv("runtime_security_config.endpoints.dd_url", "DD_RUNTIME_SECURITY_CONFIG_ENDPOINTS_DD_URL")
		cfg.BindEnv("logs_config.auto_multi_line.tokenizer_max_input_bytes", "DD_LOGS_CONFIG_AUTO_MULTI_LINE_TOKENIZER_MAX_INPUT_BYTES")
	})

	// AllKeysLowercased does not match between the implementations.
	// It turns out viper will create a "key" from the parsed yaml for all of these "empty sections"
	// because it actually sees them as leaf nodes with an empty value
	expectedKeys := []string{
		"apm_config.telemetry",
		"database_monitoring.samples",
		"logs_config.auto_multi_line",
		"logs_config.auto_multi_line.tokenizer_max_input_bytes",
		"runtime_security_config.endpoints",
		"runtime_security_config.endpoints.dd_url",
		"unknown_section.info",
	}
	expectedKeys2 := []string{
		"apm_config.telemetry.dd_url",
		"database_monitoring.samples.dd_url",
		"logs_config.auto_multi_line.tokenizer_max_input_bytes",
		"runtime_security_config.endpoints.dd_url",
		"unknown_section.info",
	}
	assert.Equal(t, expectedKeys, viperConf.AllKeysLowercased())
	assert.Equal(t, expectedKeys2, ntmConf.AllKeysLowercased())

	// AllSettings does not match either.
	// 1) viper doesn't split "auto_multi_line.tokenizer_max_input_bytes" because it comes from an env var
	// 2) ntm includes "unknown_section.info" (a bug)
	expectedSettings := map[string]interface{}{
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

	// Show the raw data structure that nodetreemodel builds.
	// Note how settings that have defaults declared in the schema (like apm_config.telemetry)
	// are only inner nodes, while the undeclared (unknown_section.info) is built as a leaf
	// This is because NTM's builder tries to match nodes from the file against the declared schema
	fmt.Printf("================\n%s\n", ntmConf.Stringify("all"))

	// Now call IsConfigured and IsSet for each section / inner node to see what each
	// implementation returns. These should all match if we want to ensure compatibility

	// not configured because known does not define a setting
	assert.False(t, viperConf.IsConfigured("apm_config.telemetry"))
	assert.False(t, ntmConf.IsConfigured("apm_config.telemetry"))

	// not configured, default is defined but configured is only true for non-default sources
	assert.False(t, viperConf.IsConfigured("database_monitoring.samples"))
	assert.False(t, ntmConf.IsConfigured("database_monitoring.samples"))

	// not configured, an env var is bound but that env var is undefined
	assert.False(t, viperConf.IsConfigured("runtime_security_config.endpoints"))
	assert.False(t, ntmConf.IsConfigured("runtime_security_config.endpoints"))

	// yes configured, because an env var is defined that contains this setting
	assert.True(t, viperConf.IsConfigured("logs_config.auto_multi_line"))
	assert.True(t, ntmConf.IsConfigured("logs_config.auto_multi_line"))

	// DIFFERENCE, ntm says true because it thinks unknown_section.info is a leaf node
	// TODO: we should change IsConfigured to ensure the leaf value is non-empty
	assert.False(t, viperConf.IsConfigured("unknown_section.info"))
	assert.True(t, ntmConf.IsConfigured("unknown_section.info"))

	// False, apm_config.telemetry.dd_url is known, but that does not define it in
	// the schema. This node is not set.
	// DIFFERENCE, ntm says true, which is a bug
	assert.False(t, viperConf.IsSet("apm_config.telemetry"))
	assert.True(t, ntmConf.IsSet("apm_config.telemetry"))

	// correct, this has a default value so it is configured
	assert.True(t, viperConf.IsSet("database_monitoring.samples"))
	assert.True(t, ntmConf.IsSet("database_monitoring.samples"))

	// DIFFERENCE, ntm says true, which is a bug
	assert.False(t, viperConf.IsSet("runtime_security_config.endpoints"))
	assert.True(t, ntmConf.IsSet("runtime_security_config.endpoints"))

	// DIFFERENCE. Viper arguably should return true here, but Viper doesn't
	// connect bound env vars to their parent settings. This bug (in viper)
	// is a good reason to encourage callers to switch to IsConfigured
	assert.False(t, viperConf.IsSet("logs_config.auto_multi_line"))
	assert.True(t, ntmConf.IsSet("logs_config.auto_multi_line"))

	// DIFFERENCE. Unclear what the correct behavior here should be since
	// this setting is unknown.
	assert.False(t, viperConf.IsSet("unknown_section.info"))
	assert.True(t, ntmConf.IsSet("unknown_section.info"))

	assert.Equal(t, nil, "WIP: test incomplete")
}
