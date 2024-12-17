// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package nodetreemodel

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test that a setting with a map value is seen as a leaf by the nodetreemodel config
func TestBuildDefaultMakesTooManyNodes(t *testing.T) {
	cfg := NewConfig("test", "", nil)
	cfg.BindEnvAndSetDefault("kubernetes_node_annotations_as_tags", map[string]string{"cluster.k8s.io/machine": "kube_machine"})
	cfg.BuildSchema()
	// Ensure the config is node based
	nodeTreeConfig, ok := cfg.(NodeTreeConfig)
	require.Equal(t, ok, true)
	// Assert that the key is a leaf node, since it was directly added by BindEnvAndSetDefault
	n, err := nodeTreeConfig.GetNode("kubernetes_node_annotations_as_tags")
	require.NoError(t, err)
	_, ok = n.(LeafNode)
	require.Equal(t, ok, true)
}

// Test that default, file, and env layers can build, get merged, and retrieve settings
func TestBuildDefaultFileAndEnv(t *testing.T) {
	configData := `network_path:
  collector:
    workers: 6
secret_backend_command: ./my_secret_fetcher.sh
`
	os.Setenv("TEST_SECRET_BACKEND_TIMEOUT", "60")
	os.Setenv("TEST_NETWORK_PATH_COLLECTOR_INPUT_CHAN_SIZE", "23456")

	cfg := NewConfig("test", "TEST", strings.NewReplacer(".", "_"))
	cfg.BindEnvAndSetDefault("network_path.collector.input_chan_size", 100000)
	cfg.BindEnvAndSetDefault("network_path.collector.processing_chan_size", 100000)
	cfg.BindEnvAndSetDefault("network_path.collector.workers", 4)
	cfg.BindEnvAndSetDefault("secret_backend_command", "")
	cfg.BindEnvAndSetDefault("secret_backend_timeout", 0)
	cfg.BindEnvAndSetDefault("server_timeout", 30)

	cfg.BuildSchema()
	err := cfg.ReadConfig(strings.NewReader(configData))
	require.NoError(t, err)

	testCases := []struct {
		description  string
		setting      string
		expectValue  interface{}
		expectIntVal int
		expectSource model.Source
	}{
		{
			description:  "nested setting from env var works",
			setting:      "network_path.collector.input_chan_size",
			expectValue:  23456,
			expectSource: model.SourceEnvVar,
		},
		{
			description:  "top-level setting from env var works",
			setting:      "secret_backend_timeout",
			expectValue:  60,
			expectSource: model.SourceEnvVar,
		},
		{
			description:  "nested setting from config file works",
			setting:      "network_path.collector.workers",
			expectValue:  6,
			expectSource: model.SourceFile,
		},
		{
			description:  "top-level setting from config file works",
			setting:      "secret_backend_command",
			expectValue:  "./my_secret_fetcher.sh",
			expectSource: model.SourceFile,
		},
		{
			description:  "nested setting from default works",
			setting:      "network_path.collector.processing_chan_size",
			expectValue:  100000,
			expectSource: model.SourceDefault,
		},
		{
			description:  "top-level setting from default works",
			setting:      "server_timeout",
			expectValue:  30,
			expectSource: model.SourceDefault,
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("case %d: setting %s", i, tc.setting), func(t *testing.T) {
			val := cfg.Get(tc.setting)
			require.Equal(t, tc.expectValue, val)
			src := cfg.GetSource(tc.setting)
			require.Equal(t, tc.expectSource, src)
		})
	}
}

func TestNewConfig(t *testing.T) {
	cfg := NewConfig("config_name", "PREFIX", nil)

	c := cfg.(*ntmConfig)

	assert.False(t, c.ready.Load())

	assert.Equal(t, "config_name", c.configName)
	assert.Equal(t, "", c.configFile)
	assert.Equal(t, "PREFIX", c.envPrefix)

	assert.NotNil(t, c.defaults)
	assert.NotNil(t, c.file)
	assert.NotNil(t, c.unknown)
	assert.NotNil(t, c.envs)
	assert.NotNil(t, c.runtime)
	assert.NotNil(t, c.localConfigProcess)
	assert.NotNil(t, c.remoteConfig)
	assert.NotNil(t, c.fleetPolicies)
	assert.NotNil(t, c.cli)

	// TODO: test SetTypeByDefaultValue and SetEnvKeyReplacer once implemented
}

// TODO: expand testing coverage once we have environment and Set() implemented
func TestBasicUsage(t *testing.T) {
	cfg := NewConfig("test", "TEST", nil)

	cfg.SetDefault("a", 1)
	cfg.BuildSchema()
	cfg.ReadConfig(strings.NewReader("a: 2"))

	assert.Equal(t, 2, cfg.Get("a"))
}

func TestSet(t *testing.T) {
	cfg := NewConfig("test", "TEST", nil)

	cfg.SetDefault("default", 0)
	cfg.SetDefault("unknown", 0)
	cfg.SetDefault("file", 0)
	cfg.SetDefault("env", 0)
	cfg.SetDefault("runtime", 0)
	cfg.SetDefault("localConfigProcess", 0)
	cfg.SetDefault("rc", 0)
	cfg.SetDefault("fleetPolicies", 0)
	cfg.SetDefault("cli", 0)

	cfg.BuildSchema()

	assert.Equal(t, 0, cfg.Get("default"))
	assert.Equal(t, 0, cfg.Get("unknown"))
	assert.Equal(t, 0, cfg.Get("file"))
	assert.Equal(t, 0, cfg.Get("env"))
	assert.Equal(t, 0, cfg.Get("runtime"))
	assert.Equal(t, 0, cfg.Get("localConfigProcess"))
	assert.Equal(t, 0, cfg.Get("rc"))
	assert.Equal(t, 0, cfg.Get("fleetPolicies"))
	assert.Equal(t, 0, cfg.Get("cli"))

	cfg.Set("unknown", 1, model.SourceUnknown)
	assert.Equal(t, 1, cfg.Get("unknown"))

	cfg.ReadConfig(strings.NewReader(`
file: 2
`))

	assert.Equal(t, 2, cfg.Get("file"))

	cfg.Set("unknown", 1, model.SourceUnknown)
	cfg.Set("env", 3, model.SourceEnvVar)
	cfg.Set("runtime", 4, model.SourceAgentRuntime)
	cfg.Set("localConfigProcess", 5, model.SourceLocalConfigProcess)
	cfg.Set("rc", 6, model.SourceRC)
	cfg.Set("fleetPolicies", 7, model.SourceFleetPolicies)
	cfg.Set("cli", 8, model.SourceCLI)

	assert.Equal(t, 0, cfg.Get("default"))
	assert.Equal(t, 1, cfg.Get("unknown"))
	assert.Equal(t, 2, cfg.Get("file"))
	assert.Equal(t, 3, cfg.Get("env"))
	assert.Equal(t, 4, cfg.Get("runtime"))
	assert.Equal(t, 5, cfg.Get("localConfigProcess"))
	assert.Equal(t, 6, cfg.Get("rc"))
	assert.Equal(t, 7, cfg.Get("fleetPolicies"))
	assert.Equal(t, 8, cfg.Get("cli"))

	assert.Equal(t, model.SourceDefault, cfg.GetSource("default"))
	assert.Equal(t, model.SourceUnknown, cfg.GetSource("unknown"))
	assert.Equal(t, model.SourceFile, cfg.GetSource("file"))
	assert.Equal(t, model.SourceEnvVar, cfg.GetSource("env"))
	assert.Equal(t, model.SourceAgentRuntime, cfg.GetSource("runtime"))
	assert.Equal(t, model.SourceLocalConfigProcess, cfg.GetSource("localConfigProcess"))
	assert.Equal(t, model.SourceRC, cfg.GetSource("rc"))
	assert.Equal(t, model.SourceFleetPolicies, cfg.GetSource("fleetPolicies"))
	assert.Equal(t, model.SourceCLI, cfg.GetSource("cli"))
}

func TestGetSource(t *testing.T) {
	cfg := NewConfig("test", "TEST", nil)
	cfg.SetDefault("a", 0)
	cfg.BuildSchema()

	assert.Equal(t, model.SourceDefault, cfg.GetSource("a"))
	cfg.Set("a", 0, model.SourceAgentRuntime)
	assert.Equal(t, model.SourceAgentRuntime, cfg.GetSource("a"))
}

func TestSetLowerSource(t *testing.T) {
	cfg := NewConfig("test", "TEST", nil)

	cfg.SetDefault("setting", 0)
	cfg.BuildSchema()

	assert.Equal(t, 0, cfg.Get("setting"))
	assert.Equal(t, model.SourceDefault, cfg.GetSource("setting"))

	cfg.Set("setting", 1, model.SourceAgentRuntime)

	assert.Equal(t, 1, cfg.Get("setting"))
	assert.Equal(t, model.SourceAgentRuntime, cfg.GetSource("setting"))

	cfg.Set("setting", 2, model.SourceFile)

	assert.Equal(t, 1, cfg.Get("setting"))
	assert.Equal(t, model.SourceAgentRuntime, cfg.GetSource("setting"))
}

func TestSetUnkownKey(t *testing.T) {
	cfg := NewConfig("test", "TEST", nil)
	cfg.BuildSchema()

	cfg.Set("unknown_key", 21, model.SourceAgentRuntime)

	assert.Nil(t, cfg.Get("unknown_key"))
	assert.Equal(t, model.SourceUnknown, cfg.GetSource("unknown_key"))
}

func TestAllSettings(t *testing.T) {
	cfg := NewConfig("test", "TEST", nil)
	cfg.SetDefault("a", 0)
	cfg.SetDefault("b.c", 0)
	cfg.SetDefault("b.d", 0)
	cfg.BuildSchema()

	cfg.ReadConfig(strings.NewReader("a: 987"))
	cfg.Set("b.c", 123, model.SourceAgentRuntime)

	expected := map[string]interface{}{
		"a": 987,
		"b": map[string]interface{}{
			"c": 123,
			"d": 0,
		},
	}
	assert.Equal(t, expected, cfg.AllSettings())
}

func TestAllSettingsWithoutDefault(t *testing.T) {
	cfg := NewConfig("test", "TEST", nil)
	cfg.SetDefault("a", 0)
	cfg.SetDefault("b.c", 0)
	cfg.SetDefault("b.d", 0)
	cfg.BuildSchema()

	cfg.ReadConfig(strings.NewReader("a: 987"))
	cfg.Set("b.c", 123, model.SourceAgentRuntime)

	expected := map[string]interface{}{
		"a": 987,
		"b": map[string]interface{}{
			"c": 123,
		},
	}
	assert.Equal(t, expected, cfg.AllSettingsWithoutDefault())
}

func TestAllSettingsBySource(t *testing.T) {
	cfg := NewConfig("test", "TEST", nil)
	cfg.SetDefault("a", 0)
	cfg.SetDefault("b.c", 0)
	cfg.SetDefault("b.d", 0)
	cfg.BuildSchema()

	cfg.ReadConfig(strings.NewReader("a: 987"))
	cfg.Set("b.c", 123, model.SourceAgentRuntime)

	expected := map[model.Source]interface{}{
		model.SourceDefault: map[string]interface{}{
			"a": 0,
			"b": map[string]interface{}{
				"c": 0,
				"d": 0,
			},
		},
		model.SourceUnknown: map[string]interface{}{},
		model.SourceFile: map[string]interface{}{
			"a": 987,
		},
		model.SourceEnvVar:        map[string]interface{}{},
		model.SourceFleetPolicies: map[string]interface{}{},
		model.SourceAgentRuntime: map[string]interface{}{
			"b": map[string]interface{}{
				"c": 123,
			},
		},
		model.SourceLocalConfigProcess: map[string]interface{}{},
		model.SourceRC:                 map[string]interface{}{},
		model.SourceCLI:                map[string]interface{}{},
	}
	assert.Equal(t, expected, cfg.AllSettingsBySource())
}

func TestIsSet(t *testing.T) {
	cfg := NewConfig("test", "TEST", nil)
	cfg.SetDefault("a", 0)
	cfg.SetDefault("b", 0)
	cfg.SetKnown("c")
	cfg.BuildSchema()

	cfg.Set("b", 123, model.SourceAgentRuntime)

	assert.True(t, cfg.IsSet("a"))
	assert.True(t, cfg.IsSet("b"))
	assert.False(t, cfg.IsSet("c"))

	assert.True(t, cfg.IsKnown("a"))
	assert.True(t, cfg.IsKnown("b"))
	assert.True(t, cfg.IsKnown("c"))

	assert.False(t, cfg.IsSet("unknown"))
	assert.False(t, cfg.IsKnown("unknown"))
}

func TestAllKeysLowercased(t *testing.T) {
	cfg := NewConfig("test", "TEST", nil)
	cfg.SetDefault("a", 0)
	cfg.SetDefault("b", 0)
	cfg.BuildSchema()

	cfg.Set("b", 123, model.SourceAgentRuntime)

	keys := cfg.AllKeysLowercased()
	sort.Strings(keys)
	assert.Equal(t, []string{"a", "b"}, keys)
}

func TestStringify(t *testing.T) {
	configData := `network_path:
  collector:
    workers: 6
secret_backend_command: ./my_secret_fetcher.sh
`
	os.Setenv("TEST_SECRET_BACKEND_TIMEOUT", "60")
	os.Setenv("TEST_NETWORK_PATH_COLLECTOR_INPUT_CHAN_SIZE", "23456")

	cfg := NewConfig("test", "TEST", strings.NewReplacer(".", "_"))
	cfg.BindEnvAndSetDefault("network_path.collector.input_chan_size", 100000)
	cfg.BindEnvAndSetDefault("network_path.collector.processing_chan_size", 100000)
	cfg.BindEnvAndSetDefault("network_path.collector.workers", 4)
	cfg.BindEnvAndSetDefault("secret_backend_command", "")
	cfg.BindEnvAndSetDefault("secret_backend_timeout", 0)
	cfg.BindEnvAndSetDefault("server_timeout", 30)

	cfg.BuildSchema()
	err := cfg.ReadConfig(strings.NewReader(configData))
	require.NoError(t, err)

	txt := cfg.(*ntmConfig).Stringify("none")
	expect := "Stringify error: invalid source: none"
	assert.Equal(t, expect, txt)

	txt = cfg.(*ntmConfig).Stringify(model.SourceDefault)
	expect = `network_path
  collector
    input_chan_size
      val:100000, source:default
    processing_chan_size
      val:100000, source:default
    workers
      val:4, source:default
secret_backend_command
  val:, source:default
secret_backend_timeout
  val:0, source:default
server_timeout
  val:30, source:default`
	assert.Equal(t, expect, txt)

	txt = cfg.(*ntmConfig).Stringify(model.SourceFile)
	expect = `network_path
  collector
    workers
      val:6, source:file
secret_backend_command
  val:./my_secret_fetcher.sh, source:file`
	assert.Equal(t, expect, txt)

	txt = cfg.(*ntmConfig).Stringify(model.SourceEnvVar)
	expect = `network_path
  collector
    input_chan_size
      val:23456, source:environment-variable
secret_backend_timeout
  val:60, source:environment-variable`
	assert.Equal(t, expect, txt)

	txt = cfg.(*ntmConfig).Stringify("root")
	expect = `network_path
  collector
    input_chan_size
      val:23456, source:environment-variable
    processing_chan_size
      val:100000, source:default
    workers
      val:6, source:file
secret_backend_command
  val:./my_secret_fetcher.sh, source:file
secret_backend_timeout
  val:60, source:environment-variable
server_timeout
  val:30, source:default`
	assert.Equal(t, expect, txt)
}

func TestUnsetForSource(t *testing.T) {
	// env source, highest priority
	os.Setenv("TEST_NETWORK_PATH_COLLECTOR_INPUT_CHAN_SIZE", "23456")
	os.Setenv("TEST_NETWORK_PATH_COLLECTOR_PATHTEST_CONTEXTS_LIMIT", "654321")
	os.Setenv("TEST_NETWORK_PATH_COLLECTOR_PROCESSING_CHAN_SIZE", "78900")
	// file source, medium priority
	configData := `network_path:
  collector:
    workers: 6
    pathtest_contexts_limit: 43210
    processing_chan_size: 45678`
	// default source, lowest priority
	cfg := NewConfig("test", "TEST", strings.NewReplacer(".", "_"))
	cfg.BindEnvAndSetDefault("network_path.collector.input_chan_size", 100000)
	cfg.BindEnvAndSetDefault("network_path.collector.pathtest_contexts_limit", 100000)
	cfg.BindEnvAndSetDefault("network_path.collector.processing_chan_size", 100000)
	cfg.BindEnvAndSetDefault("network_path.collector.workers", 4)

	cfg.BuildSchema()
	err := cfg.ReadConfig(strings.NewReader(configData))
	require.NoError(t, err)

	// The merged config
	txt := cfg.(*ntmConfig).Stringify("root")
	expect := `network_path
  collector
    input_chan_size
      val:23456, source:environment-variable
    pathtest_contexts_limit
      val:654321, source:environment-variable
    processing_chan_size
      val:78900, source:environment-variable
    workers
      val:6, source:file`
	assert.Equal(t, expect, txt)

	// No change if source doesn't match
	cfg.UnsetForSource("network_path.collector.input_chan_size", model.SourceFile)
	assert.Equal(t, expect, txt)

	// No change if setting is not a leaf
	cfg.UnsetForSource("network_path", model.SourceEnvVar)
	assert.Equal(t, expect, txt)

	// No change if setting is not found
	cfg.UnsetForSource("network_path.unknown", model.SourceEnvVar)
	assert.Equal(t, expect, txt)

	// Remove a setting from the env source, nothing in the file source, it goes to default
	cfg.UnsetForSource("network_path.collector.input_chan_size", model.SourceEnvVar)
	txt = cfg.(*ntmConfig).Stringify("root")
	expect = `network_path
  collector
    input_chan_size
      val:100000, source:default
    pathtest_contexts_limit
      val:654321, source:environment-variable
    processing_chan_size
      val:78900, source:environment-variable
    workers
      val:6, source:file`
	assert.Equal(t, expect, txt)

	// Remove a setting from the file source, it goes to default
	cfg.UnsetForSource("network_path.collector.workers", model.SourceFile)
	txt = cfg.(*ntmConfig).Stringify("root")
	expect = `network_path
  collector
    input_chan_size
      val:100000, source:default
    pathtest_contexts_limit
      val:654321, source:environment-variable
    processing_chan_size
      val:78900, source:environment-variable
    workers
      val:4, source:default`
	assert.Equal(t, expect, txt)

	// Removing a setting from the env source, it goes to file source
	cfg.UnsetForSource("network_path.collector.processing_chan_size", model.SourceEnvVar)
	txt = cfg.(*ntmConfig).Stringify("root")
	expect = `network_path
  collector
    input_chan_size
      val:100000, source:default
    pathtest_contexts_limit
      val:654321, source:environment-variable
    processing_chan_size
      val:45678, source:file
    workers
      val:4, source:default`
	assert.Equal(t, expect, txt)

	// Then remove it from the file source as well, leaving the default source
	cfg.UnsetForSource("network_path.collector.processing_chan_size", model.SourceFile)
	txt = cfg.(*ntmConfig).Stringify("root")
	expect = `network_path
  collector
    input_chan_size
      val:100000, source:default
    pathtest_contexts_limit
      val:654321, source:environment-variable
    processing_chan_size
      val:100000, source:default
    workers
      val:4, source:default`
	assert.Equal(t, expect, txt)

	// Check the file layer in isolation
	fileTxt := cfg.(*ntmConfig).Stringify(model.SourceFile)
	fileExpect := `network_path
  collector
    pathtest_contexts_limit
      val:43210, source:file`
	assert.Equal(t, fileExpect, fileTxt)

	// Removing from the file source first does not change the merged value, because it uses env layer
	cfg.UnsetForSource("network_path.collector.pathtest_contexts_limit", model.SourceFile)
	assert.Equal(t, expect, txt)

	// But the file layer itself has been modified
	fileTxt = cfg.(*ntmConfig).Stringify(model.SourceFile)
	fileExpect = `network_path
  collector`
	assert.Equal(t, fileExpect, fileTxt)

	// Finally, remove it from the env layer
	cfg.UnsetForSource("network_path.collector.pathtest_contexts_limit", model.SourceEnvVar)
	txt = cfg.(*ntmConfig).Stringify("root")
	expect = `network_path
  collector
    input_chan_size
      val:100000, source:default
    pathtest_contexts_limit
      val:100000, source:default
    processing_chan_size
      val:100000, source:default
    workers
      val:4, source:default`
	assert.Equal(t, expect, txt)
}

func TestMergeFleetPolicy(t *testing.T) {
	config := NewConfig("test", "TEST", strings.NewReplacer(".", "_")) // nolint: forbidigo
	config.SetConfigType("yaml")
	config.SetDefault("foo", "")
	config.BuildSchema()
	config.Set("foo", "bar", model.SourceFile)

	file, err := os.CreateTemp("", "datadog.yaml")
	assert.NoError(t, err, "failed to create temporary file: %w", err)
	file.Write([]byte("foo: baz"))
	err = config.MergeFleetPolicy(file.Name())
	assert.NoError(t, err)

	assert.Equal(t, "baz", config.Get("foo"))
	assert.Equal(t, model.SourceFleetPolicies, config.GetSource("foo"))
}
