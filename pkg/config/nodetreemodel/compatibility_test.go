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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/helper"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

// constructNtmConfig builds a nodetreemodel config for tests, applying the optional setup
// function and loading the given YAML content (or reading from disk when content is empty).
func constructNtmConfig(content string, dynamicSchema bool, setupFunc func(model.Setup)) model.BuildableConfig {
	conf := NewNodeTreeConfig("datadog", "DD", strings.NewReplacer(".", "_"))

	if dynamicSchema {
		conf.SetTestOnlyDynamicSchema(true)
	}
	if setupFunc != nil {
		setupFunc(conf)
	}

	conf.BuildSchema()

	if len(content) > 0 {
		conf.SetConfigType("yaml")
		conf.ReadConfig(bytes.NewBuffer([]byte(content)))
	} else {
		conf.ReadInConfig()
	}

	return conf
}

func TestAllFlattenedSettingsWithSequenceID(t *testing.T) {
	t.Setenv("DD_MY_FEATURE_ENABLED", "true")
	t.Setenv("DD_PORT", "345")
	conf := constructNtmConfig("", false, func(cfg model.Setup) {
		cfg.BindEnvAndSetDefault("my_feature.enabled", false)
		cfg.BindEnvAndSetDefault("port", 0)
	})

	settings, _ := conf.AllFlattenedSettingsWithSequenceID()

	expectmap := map[string]interface{}{
		"my_feature.enabled": true,
		"port":               345,
	}
	assert.Equal(t, expectmap, settings)
}

func TestAllFlattenedSettingsWithSequenceIDDottedMapKeys(t *testing.T) {
	conf := constructNtmConfig("", false, func(cfg model.Setup) {
		cfg.BindEnvAndSetDefault("additional_endpoints", map[string][]string{})
	})

	endpoints := map[string]interface{}{
		"https://url1.com": []interface{}{"api_key_1"},
		"https://url2.eu":  []interface{}{"api_key_2"},
	}
	conf.Set("additional_endpoints", endpoints, model.SourceFile)

	settings, _ := conf.AllFlattenedSettingsWithSequenceID()

	assert.Contains(t, settings, "additional_endpoints")
	for key := range settings {
		assert.False(t, strings.HasPrefix(key, "additional_endpoints."), "unexpected dotted child key in map: %s", key)
	}
}

func TestAllFlattenedSettingsWithSequenceIDKnownLeaf(t *testing.T) {
	conf := constructNtmConfig("", false, func(cfg model.Setup) {
		cfg.BindEnvAndSetDefault("apm_config.foo", "")
		cfg.BindEnvAndSetDefault("apm_config.bar.baz", "")
		cfg.BindEnvAndSetDefault("proxy.http", "")
	})

	settings, _ := conf.AllFlattenedSettingsWithSequenceID()

	keys := slices.Collect(maps.Keys(settings))
	expectedKeys := []string{"apm_config.foo", "apm_config.bar.baz", "proxy.http"}
	assert.ElementsMatch(t, expectedKeys, keys)
	assert.NotContains(t, keys, "apm_config")
	assert.NotContains(t, keys, "apm_config.bar")
}

func TestAllFlattenedSettingsWithSequenceIDUnknownParentChild(t *testing.T) {
	conf := constructNtmConfig("", false, func(cfg model.Setup) {
		cfg.BindEnvAndSetDefault("proxy.http", "")
	})

	// Reading unknown keys tracks them.
	_ = conf.Get("unknown_section")
	_ = conf.Get("unknown_section.info")

	settings, _ := conf.AllFlattenedSettingsWithSequenceID()

	keys := slices.Collect(maps.Keys(settings))
	expectedKeys := []string{"proxy.http", "unknown_section", "unknown_section.info"}
	assert.ElementsMatch(t, expectedKeys, keys)
}

func TestGetEnvVarsBindings(t *testing.T) {
	dataYaml := `unknown_setting: 123`

	t.Run("With BindEnvAndSetDefault", func(t *testing.T) {
		conf := constructNtmConfig(dataYaml, false, func(cfg model.Setup) {
			cfg.BindEnvAndSetDefault("port", "", "TEST_PORT")
			cfg.BindEnvAndSetDefault("host", "", "TEST_HOST")
			cfg.BindEnvAndSetDefault("log.level", "", "TEST_LOG_LEVEL")
		})

		envVars := conf.GetEnvVars()
		sort.Strings(envVars)

		assert.Equal(t, []string{"TEST_HOST", "TEST_LOG_LEVEL", "TEST_PORT"}, envVars)
	})

	t.Run("Without BindEnvAndSetDefault", func(t *testing.T) {
		conf := constructNtmConfig(dataYaml, false, nil)
		assert.Empty(t, conf.GetEnvVars(), "should return no env vars without BindEnvAndSetDefault")
	})

	t.Run("With EnvPrefix", func(t *testing.T) {
		conf := constructNtmConfig(dataYaml, false, func(cfg model.Setup) {
			cfg.SetEnvPrefix("MYAPP")
			cfg.BindEnvAndSetDefault("port", "")
		})

		assert.Contains(t, conf.GetEnvVars(), "MYAPP_PORT", "should apply EnvPrefix")
	})

	t.Run("With EnvKeyReplacer", func(t *testing.T) {
		conf := constructNtmConfig(dataYaml, false, func(cfg model.Setup) {
			cfg.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
			cfg.BindEnvAndSetDefault("log.level", "")
		})

		// Default prefix is "DD" when initializing the config
		assert.Contains(t, conf.GetEnvVars(), "DD_LOG_LEVEL", "should apply EnvKeyReplacer")
	})

	t.Run("With EnvPrefix and EnvKeyReplacer", func(t *testing.T) {
		conf := constructNtmConfig(dataYaml, false, func(cfg model.Setup) {
			cfg.SetEnvPrefix("MYAPP")
			cfg.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
			cfg.BindEnvAndSetDefault("db.connection.url", "")
		})

		assert.Contains(t, conf.GetEnvVars(), "MYAPP_DB_CONNECTION_URL", "should apply prefix and replacer")
	})

	t.Run("Adding an unknown setting in the yaml", func(t *testing.T) {
		conf := constructNtmConfig(dataYaml, false, func(cfg model.Setup) {
			cfg.SetDefault("HOST", "localhost")
			cfg.BindEnvAndSetDefault("log_level", "")
		})

		envVars := conf.GetEnvVars()
		sort.Strings(envVars)

		assert.Equal(t, []string{"DD_LOG_LEVEL"}, envVars, "should return only known env vars")
	})

	t.Run("Duplicate env vars", func(t *testing.T) {
		conf := constructNtmConfig(dataYaml, false, func(cfg model.Setup) {
			cfg.BindEnvAndSetDefault("test", "", "ABC")
			cfg.BindEnvAndSetDefault("test2", "", "ABC")
			cfg.BindEnvAndSetDefault("test3", "")
		})

		envVars := conf.GetEnvVars()
		sort.Strings(envVars)

		assert.Equal(t, []string{"ABC", "DD_TEST3"}, envVars, "should return only known env vars")
	})
}

func TestEmptyConfigSection(t *testing.T) {
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
	// apm_config.telemetry        - empty section declared only in the yaml (neither default nor env var)
	// database_monitoring.samples - defines a default
	// logs_config.auto_multi_line - bind an env var and assign a value to that env var
	// runtime_security_config.endpoints - bind an env var but leave that env var undefined
	// unknown_section.info        - undefined, neither default nor env var, only shows up in the config.yaml
	// additional_endpoints        - defines a default, does not appear in the file

	t.Setenv("DD_LOGS_CONFIG_AUTO_MULTI_LINE_TOKENIZER_MAX_INPUT_BYTES", "100")

	conf := constructNtmConfig(dataYaml, true, func(cfg model.Setup) {
		cfg.SetDefault("database_monitoring.samples.dd_url", "")
		cfg.BindEnvAndSetDefault("runtime_security_config.endpoints.dd_url", "", "DD_RUNTIME_SECURITY_CONFIG_ENDPOINTS_DD_URL")
		cfg.BindEnvAndSetDefault("logs_config.auto_multi_line.tokenizer_max_input_bytes", "", "DD_LOGS_CONFIG_AUTO_MULTI_LINE_TOKENIZER_MAX_INPUT_BYTES")
		cfg.BindEnvAndSetDefault("additional_endpoints", map[string][]string{})
	})

	expectedKeys := []string{
		"additional_endpoints",
		"apm_config.telemetry",
		"database_monitoring.samples.dd_url",
		//"logs_config.auto_multi_line",
		"logs_config.auto_multi_line.tokenizer_max_input_bytes",
		//"runtime_security_config.endpoints",
		"runtime_security_config.endpoints.dd_url",
		"unknown_section.info",
	}
	assert.Equal(t, expectedKeys, conf.AllKeysLowercased())

	expectedSettings := map[string]interface{}{
		"additional_endpoints": map[string][]string{},
		"runtime_security_config": map[string]interface{}{
			"endpoints": map[string]interface{}{
				"dd_url": "",
			},
		},
		"apm_config": map[string]interface{}{
			"telemetry": nil,
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
	assert.Equal(t, expectedSettings, conf.AllSettings())

	////////////
	// tests for IsConfigured

	// not configured because the empty yaml section does not define a value
	assert.False(t, conf.IsConfigured("apm_config.telemetry"))

	// not configured by the file nor env var
	assert.False(t, conf.IsConfigured("apm_config.telemetry.enabled"))

	// not configured, because only default is defined
	assert.False(t, conf.IsConfigured("database_monitoring.samples"))

	// not configured, an env var is bound but that env var is undefined
	assert.False(t, conf.IsConfigured("runtime_security_config.endpoints"))

	// yes configured, because an env var is defined that contains this setting
	assert.True(t, conf.IsConfigured("logs_config.auto_multi_line"))

	// not configured, unknown section
	assert.False(t, conf.IsConfigured("unknown_section.info"))

	// not configured, because only default is defined
	assert.False(t, conf.IsConfigured("additional_endpoints"))

	////////////
	// tests for HasSection

	// HasSection true for an empty section
	assert.True(t, conf.HasSection("apm_config.telemetry"))

	// False because this setting isn't defined at all
	assert.False(t, conf.HasSection("apm_config.telemetry.enabled"))

	// HasSection true for an empty section
	assert.True(t, conf.HasSection("database_monitoring.samples"))

	// HasSection true for an empty section
	assert.True(t, conf.HasSection("runtime_security_config.endpoints"))

	// HasSection true because the section has data
	assert.True(t, conf.HasSection("logs_config.auto_multi_line"))

	// HasSection true for an empty section, even though it is unknown
	assert.True(t, conf.HasSection("unknown_section.info"))
}

func TestEmptyLeafSetting(t *testing.T) {
	dataYaml := `
otlp_config:
  logs:
    enabled:
`
	conf := constructNtmConfig(dataYaml, true, func(cfg model.Setup) {
		cfg.BindEnvAndSetDefault("otlp_config.logs.enabled", true)
	})

	// not configured because the setting's value is nil
	assert.False(t, conf.IsConfigured("otlp_config.logs.enabled"))

	// HasSection is always false for leaf settings
	assert.False(t, conf.HasSection("otlp_config.logs.enabled"))

	// NTM merges layers, replacing the missing file data using the default layer
	expected := map[string]interface{}{"enabled": true}
	assert.Equal(t, expected, conf.Get("otlp_config.logs"))

	// returns true, because of the default value
	assert.Equal(t, true, conf.GetBool("otlp_config.logs.enabled"))

	// Even without specifying the type, using Get instead of GetBool
	assert.Equal(t, true, conf.Get("otlp_config.logs.enabled"))
}

func TestConflictDataType(t *testing.T) {
	var yamlPayload = `
a: orange
c: 1234
`
	conf := constructNtmConfig(yamlPayload, true, func(cfg model.Setup) {
		cfg.SetDefault("a", "apple")
		cfg.SetDefault("c.d", true)
	})

	assert.Equal(t, 1234, conf.Get("c"))
	assert.Equal(t, nil, conf.Get("c.d"))
}

func TestTimeDuration(t *testing.T) {
	conf := constructNtmConfig("", false, func(cfg model.Setup) {
		cfg.SetDefault("provider.interval", 5*time.Second)
		cfg.SetDefault("lookup_timeout", 30*time.Millisecond)
	})
	assert.Equal(t, 5*time.Second, conf.GetDuration("provider.interval"))
	assert.Equal(t, 30*time.Millisecond, conf.GetDuration("lookup_timeout"))

	// refuse to convert time.Duration to int64
	assert.Equal(t, int64(0), conf.GetInt64("lookup_timeout"))

	assert.Equal(t, 30*time.Millisecond, conf.Get("lookup_timeout"))
}

func TestReadConfigReset(t *testing.T) {
	initialYAML := `port: 1234`
	overrideYAML := `host: localhost`

	conf := constructNtmConfig(initialYAML, true, func(cfg model.Setup) {
		cfg.BindEnvAndSetDefault("port", 0)
		cfg.BindEnvAndSetDefault("host", "")
	})

	assert.Equal(t, 1234, conf.GetInt("port"))
	assert.False(t, conf.IsConfigured("host"))

	// Now use ReadConfig to reset with only "host"
	conf.SetConfigType("yaml")
	err := conf.ReadConfig(bytes.NewBuffer([]byte(overrideYAML)))
	assert.NoError(t, err)

	// After ReadConfig, "port" should be gone, "host" should be set
	assert.False(t, conf.IsConfigured("port"), "should have cleared previous config")
	assert.Equal(t, "localhost", conf.GetString("host"))
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

	conf := NewNodeTreeConfig("datadog", "DD", strings.NewReplacer(".", "_"))
	conf.SetTestOnlyDynamicSchema(true)
	conf.SetConfigFile(configAPath)
	conf.BuildSchema()

	err = conf.ReadInConfig()
	assert.NoError(t, err)

	assert.Equal(t, 8123, conf.GetInt("port"))
	assert.False(t, conf.IsConfigured("host"))

	// Update config file to configB (overwrites configA)
	conf.SetConfigFile(configBPath)

	err = conf.ReadInConfig()
	assert.NoError(t, err)

	assert.False(t, conf.IsConfigured("port"), "should have cleared previous config")
	// "host" should now be available
	assert.Equal(t, "localhost", conf.GetString("host"))
}

func TestReadInConfigExactError(t *testing.T) {
	// Invalid YAML that will fail to parse
	dataYaml := `site:datadoghq.eu
`
	conf := constructNtmConfig(dataYaml, false, func(cfg model.Setup) {
		cfg.BindEnvAndSetDefault("site", "datadoghq.com")
	})

	// Should return "Config File Not Found" when a parsing error is encountered
	err := conf.ReadInConfig()
	assert.ErrorIs(t, err, model.ErrConfigFileNotFound)

	assert.Equal(t, "datadoghq.com", conf.GetString("site"))
}

func TestEnvVarsSubfields(t *testing.T) {
	t.Run("Subsettings are merged with env vars", func(t *testing.T) {
		data, _ := json.Marshal(map[string]string{"a": "apple"})
		t.Setenv("TEST_MY_FEATURE_INFO_TARGETS", string(data))

		configData := `
my_feature:
  info:
    enabled: true
`
		conf := constructNtmConfig(configData, false, func(cfg model.Setup) {
			cfg.BindEnvAndSetDefault("my_feature.info.name", "feat")
			cfg.BindEnvAndSetDefault("my_feature.info.enabled", false)
			cfg.BindEnvAndSetDefault("my_feature.info.version", "v2")
			cfg.BindEnvAndSetDefault("my_feature.info.targets", "", "TEST_MY_FEATURE_INFO_TARGETS")
		})

		fields := conf.GetSubfields("my_feature.info")
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
	conf := constructNtmConfig(configData, false, func(cfg model.Setup) {
		// default wins over invalid file
		cfg.BindEnvAndSetDefault("fruit.apple.core.seeds", 2)

		// fruit.banana.peel.color is intentionally left undeclared: it only appears
		// in the file (as an empty section) and has no default

		// env wins over file
		cfg.BindEnvAndSetDefault("fruit.cherry.seed.num", 0)
	})

	expectAppleMap := map[string]interface{}{
		"core": map[string]interface{}{
			"seeds": 2,
		},
	}

	assert.Equal(t, expectAppleMap, conf.Get("fruit.apple"))
	assert.Equal(t, nil, conf.Get("fruit.banana"))
	assert.Equal(t, 2, conf.GetInt("fruit.apple.core.seeds"))
	assert.Equal(t, "", conf.GetString("fruit.banana.peel.color"))
	assert.Equal(t, 5, conf.GetInt("fruit.cherry.seed.num"))
	assert.Equal(t, 12, conf.GetInt("fruit.donut.dozen"))
}

func TestConfigUsesDotSeparatedFields(t *testing.T) {
	configData := `
my_feature.info.enabled: true
second_feature:
  info.enabled: true
additional_endpoints:
  https://url1.com:
    - my_api_key
`
	conf := constructNtmConfig(configData, false, func(cfg model.Setup) {
		cfg.BindEnvAndSetDefault("my_feature.info.name", "feat")
		cfg.BindEnvAndSetDefault("my_feature.info.enabled", false)
		cfg.BindEnvAndSetDefault("my_feature.info.version", "v2")
		cfg.BindEnvAndSetDefault("second_feature.info.enabled", false)
		cfg.BindEnvAndSetDefault("additional_endpoints", map[string][]string{})
	})

	assert.Equal(t, true, conf.Get("my_feature.info.enabled"))
	assert.Equal(t, true, conf.Get("second_feature.info.enabled"))

	expectEndpoints := map[string][]string{
		"https://url1.com": {"my_api_key"},
	}
	assert.Equal(t, expectEndpoints, conf.GetStringMapStringSlice("additional_endpoints"))
}

func TestGetViperCombineInvalidFileData(t *testing.T) {
	// The setting in the yaml file has the wrong shape.
	// It is a list of an object, but it is supposed to not be a list.
	// The implementation should handle this predictably: when merging conflicts higher
	// layers have branches kept, so the invalid file data is kept rather than the defaults.
	configData := `network_path:
  collector:
    - input_chan_size: 23456
`
	// Two settings at path, but the file source has the wrong shape
	conf := constructNtmConfig(configData, false, func(cfg model.Setup) {
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

	assert.Equal(t, expectCollector, conf.Get("network_path.collector"))
	assert.Equal(t, expectCollector, helper.GetViperCombine(conf, "network_path.collector"))
	assert.Equal(t, expectCollector, conf.AllSettings()["network_path"].(map[string]interface{})["collector"])

	// Test parent element as well
	assert.Equal(t, expectNetworkPath, helper.GetViperCombine(conf, "network_path"))
}

func TestCompareEmptyLeafSetting(t *testing.T) {
	dataYaml := `
otlp_config:
  logs:
    enabled:
`
	conf := constructNtmConfig(dataYaml, true, func(cfg model.Setup) {
		cfg.BindEnvAndSetDefault("otlp_config.logs.enabled", true)
	})

	// not configured because the setting's value is nil
	assert.False(t, conf.IsConfigured("otlp_config.logs.enabled"))

	// HasSection is always false for leaf settings
	assert.False(t, conf.HasSection("otlp_config.logs.enabled"))

	expected := map[string]interface{}(map[string]interface{}{"enabled": true})
	assert.Equal(t, expected, conf.Get("otlp_config.logs"))

	// But both return true, because of the default value
	assert.Equal(t, true, conf.GetBool("otlp_config.logs.enabled"))

	// Even without specifying the type, using Get instead of GetBool
	assert.Equal(t, true, conf.Get("otlp_config.logs.enabled"))
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

	conf := constructNtmConfig("", false, func(cfg model.Setup) {
		cfg.SetDefault("some.setting", "default_value")
		cfg.SetConfigFile(tempfile.Name())
	})

	fmt.Printf("%v\n", conf.GetAllSources("some.setting"))
	conf.Set("some.setting", "runtime_value", model.SourceAgentRuntime)
	conf.Set("some.setting", "process_value", model.SourceLocalConfigProcess)
	conf.Set("some.setting", "RC_value", model.SourceRC)

	assert.Equal(t, "RC_value", conf.GetString("some.setting"))

	conf.UnsetForSource("some.setting", model.SourceRC)
	assert.Equal(t, "runtime_value", conf.GetString("some.setting"))

	conf.UnsetForSource("some.setting", model.SourceAgentRuntime)
	assert.Equal(t, "process_value", conf.GetString("some.setting"))

	conf.UnsetForSource("some.setting", model.SourceLocalConfigProcess)
	assert.Equal(t, "file_value", conf.GetString("some.setting"))

	conf.UnsetForSource("some.setting", model.SourceFile)
	assert.Equal(t, "default_value", conf.GetString("some.setting"))

	conf.UnsetForSource("some.setting", model.SourceDefault)
	assert.Equal(t, "", conf.GetString("some.setting"))
}
