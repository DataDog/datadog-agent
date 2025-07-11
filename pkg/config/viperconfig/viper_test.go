// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package viperconfig

import (
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config/model"
)

func TestConcurrencySetGet(t *testing.T) {
	config := NewViperConfig("test", "DD", strings.NewReplacer(".", "_")) // nolint: forbidigo

	var wg sync.WaitGroup

	wg.Add(2)
	go func() {
		defer wg.Done()
		for n := 0; n <= 1000; n++ {
			config.GetString("foo")
		}
	}()
	go func() {
		defer wg.Done()
		for n := 0; n <= 1000; n++ {
			config.SetWithoutSource("foo", "bar")
		}
	}()

	wg.Wait()
	assert.Equal(t, config.GetString("foo"), "bar")
}

func TestConcurrencyUnmarshalling(_ *testing.T) {
	config := NewViperConfig("test", "DD", strings.NewReplacer(".", "_")) // nolint: forbidigo

	config.SetDefault("foo", map[string]string{})
	config.SetDefault("BAR", "test")
	config.SetDefault("baz", "test")

	var wg sync.WaitGroup

	wg.Add(2)
	getter := func() {
		defer wg.Done()
		for n := 0; n <= 1000; n++ {
			config.GetStringMapString("foo")
		}
	}
	go getter()
	go getter()

	go func() {
		wg.Wait()
	}()
}

func TestGetConfigEnvVars(t *testing.T) {
	config := NewViperConfig("test", "DD", strings.NewReplacer(".", "_")) // nolint: forbidigo

	config.BindEnv("app_key")
	assert.Contains(t, config.GetEnvVars(), "DD_APP_KEY")
	config.BindEnv("logs_config.run_path")
	assert.Contains(t, config.GetEnvVars(), "DD_LOGS_CONFIG_RUN_PATH")

	config.BindEnv("config_option", "DD_CONFIG_OPTION")
	assert.Contains(t, config.GetEnvVars(), "DD_CONFIG_OPTION")
}

// check for de-duplication of environment variables by declaring two
// config parameters using DD_CONFIG_OPTION, and asserting that
// GetConfigVars only returns that env var once.
func TestGetConfigEnvVarsDedupe(t *testing.T) {
	config := NewViperConfig("test", "DD", strings.NewReplacer(".", "_")) // nolint: forbidigo

	config.BindEnv("config_option_1", "DD_CONFIG_OPTION")
	config.BindEnv("config_option_2", "DD_CONFIG_OPTION")
	count := 0
	for _, v := range config.GetEnvVars() {
		if v == "DD_CONFIG_OPTION" {
			count++
		}
	}
	assert.Equal(t, 1, count)
}

func TestSet(t *testing.T) {
	config := NewViperConfig("test", "DD", strings.NewReplacer(".", "_")) // nolint: forbidigo
	config.Set("foo", "bar", model.SourceFile)
	config.Set("foo", "baz", model.SourceEnvVar)
	config.Set("foo", "qux", model.SourceAgentRuntime)
	config.Set("foo", "quux", model.SourceRC)
	config.Set("foo", "corge", model.SourceCLI)

	layers := config.AllSettingsBySource()

	assert.Equal(t, layers[model.SourceFile], map[string]interface{}{"foo": "bar"})
	assert.Equal(t, layers[model.SourceEnvVar], map[string]interface{}{"foo": "baz"})
	assert.Equal(t, layers[model.SourceAgentRuntime], map[string]interface{}{"foo": "qux"})
	assert.Equal(t, layers[model.SourceRC], map[string]interface{}{"foo": "quux"})
	assert.Equal(t, layers[model.SourceCLI], map[string]interface{}{"foo": "corge"})

	assert.Equal(t, config.Get("foo"), "corge")
}

func TestGetSource(t *testing.T) {
	config := NewViperConfig("test", "DD", strings.NewReplacer(".", "_")) // nolint: forbidigo
	config.Set("foo", "bar", model.SourceFile)
	config.Set("foo", "baz", model.SourceEnvVar)
	assert.Equal(t, model.SourceEnvVar, config.GetSource("foo"))
}

func TestIsKnown(t *testing.T) {
	testCases := []struct {
		setDefault bool
		setEnv     bool
		setKnown   bool
		expected   bool
	}{
		{false, false, false, false},
		{false, false, true, true},
		{false, true, false, true},
		{false, true, true, true},
		{true, false, false, true},
		{true, false, true, true},
		{true, true, false, true},
		{true, true, true, true},
	}

	configDefault := "somedefault"
	configEnv := "SOME_ENV"

	for _, tc := range testCases {
		testName := "isknown"
		if tc.setKnown {
			testName += "-known"
		}
		if tc.setDefault {
			testName += "-default"
		}
		if tc.setEnv {
			testName += "-env"
		}
		t.Run(testName, func(t *testing.T) {
			for _, configName := range []string{"foo", "BAR", "BaZ", "foo_BAR", "foo.BAR", "foo.BAR.baz"} {
				config := NewViperConfig("test", "DD", strings.NewReplacer(".", "_")) // nolint: forbidigo

				if tc.setKnown {
					config.SetKnown(configName)
				}
				if tc.setDefault {
					config.SetDefault(configName, configDefault)
				}
				if tc.setEnv {
					config.BindEnv(configName, configEnv)
				}

				assert.Equal(t, tc.expected, config.IsKnown(configName))
			}
		})
	}
}

func TestAllFileSettingsWithoutDefault(t *testing.T) {
	config := NewViperConfig("test", "DD", strings.NewReplacer(".", "_")) // nolint: forbidigo
	config.Set("foo", "bar", model.SourceFile)
	config.Set("baz", "qux", model.SourceFile)
	config.UnsetForSource("foo", model.SourceFile)
	assert.Equal(
		t,
		map[string]interface{}{
			"baz": "qux",
		},
		config.AllSettingsWithoutDefault(),
	)
}

func TestSourceFileReadConfig(t *testing.T) {
	config := NewViperConfig("test", "DD", strings.NewReplacer(".", "_")) // nolint: forbidigo
	yamlExample := []byte(`
foo: bar
`)

	tempfile, err := os.CreateTemp("", "test-*.yaml")
	assert.NoError(t, err, "failed to create temporary file")
	tempfile.Write(yamlExample)
	defer os.Remove(tempfile.Name())

	config.SetConfigFile(tempfile.Name())
	config.ReadInConfig()

	assert.Equal(t, "bar", config.Get("foo"))
	assert.Equal(t, model.SourceFile, config.GetSource("foo"))
	assert.Equal(t, map[string]interface{}{"foo": "bar"}, config.AllSettingsWithoutDefault())
}

func TestNotification(t *testing.T) {
	config := NewViperConfig("test", "DD", strings.NewReplacer(".", "_")) // nolint: forbidigo

	notifications1 := make(chan string, 3)
	config.OnUpdate(func(key string, _, _ any, _ uint64) {
		notifications1 <- key
	})

	config.Set("foo", "bar", model.SourceFile)

	notifications2 := make(chan string, 2)
	config.OnUpdate(func(key string, _, _ any, _ uint64) {
		notifications2 <- key
	})

	config.Set("foo", "bar2", model.SourceFile)
	config.Set("foo2", "bar2", model.SourceFile)

	collected1 := []string{}
	for i := 0; i < 3; i++ {
		select {
		case key := <-notifications1:
			collected1 = append(collected1, key)
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for notification %d for listener 1", i+1)
		}
	}
	assert.Equal(t, []string{"foo", "foo", "foo2"}, collected1)

	collected2 := []string{}
	for i := 0; i < 2; i++ {
		select {
		case key := <-notifications2:
			collected2 = append(collected2, key)
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for notification %d for listener 2", i+1)
		}
	}
	assert.Equal(t, []string{"foo", "foo2"}, collected2)
}

func TestNotificationNoChange(t *testing.T) {
	config := NewViperConfig("test", "DD", strings.NewReplacer(".", "_")) // nolint: forbidigo
	updatedKeyCB1 := []string{}
	notifications := make(chan string, 10)
	config.OnUpdate(func(key string, _, newValue any, _ uint64) {
		notifications <- key + ":" + newValue.(string)
	})

	config.Set("foo", "bar", model.SourceFile)
	for len(updatedKeyCB1) < 1 {
		select {
		case key := <-notifications:
			updatedKeyCB1 = append(updatedKeyCB1, key)
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for notification")
		}
	}
	assert.Equal(t, []string{"foo:bar"}, updatedKeyCB1)

	config.Set("foo", "bar", model.SourceFile)
	for len(updatedKeyCB1) < 1 {
		select {
		case <-notifications:
			t.Fatalf("received unexpected notification")
		case <-time.After(2 * time.Second):
		}
	}
	assert.Equal(t, []string{"foo:bar"}, updatedKeyCB1)

	config.Set("foo", "baz", model.SourceAgentRuntime)
	for len(updatedKeyCB1) < 2 {
		select {
		case key := <-notifications:
			updatedKeyCB1 = append(updatedKeyCB1, key)
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for notification")
		}
	}
	assert.Equal(t, []string{"foo:bar", "foo:baz"}, updatedKeyCB1)

	config.Set("foo", "bar2", model.SourceFile)
	assert.Equal(t, []string{"foo:bar", "foo:baz"}, updatedKeyCB1)
}

func TestCheckKnownKey(t *testing.T) {
	config := NewViperConfig("test", "DD", strings.NewReplacer(".", "_")).(*safeConfig) // nolint: forbidigo

	config.SetKnown("foo")
	config.Get("foo")
	assert.Empty(t, config.unknownKeys)

	assert.NotContains(t, config.unknownKeys, "foobar")
	config.Get("foobar")
	assert.Contains(t, config.unknownKeys, "foobar")

	config.Get("foobar")
	assert.Contains(t, config.unknownKeys, "foobar")
}

func TestExtraConfig(t *testing.T) {
	config := NewViperConfig("test", "DD", strings.NewReplacer(".", "_")) // nolint: forbidigo

	confs := []struct {
		name    string
		content string
		file    *os.File
	}{
		{
			name:    "datadog",
			content: "api_key:",
		},
		{
			name: "extra1",
			content: `api_key: abcdef
site: datadoghq.eu
proxy:
    https: https:proxyserver1`},
		{
			name: "extra2",
			content: `proxy:
    http: http:proxyserver2`},
	}

	// write configs into temp files
	for index, conf := range confs {
		file, err := os.CreateTemp("", conf.name+"-*.yaml")
		assert.NoError(t, err, "failed to create temporary file: %w", err)
		file.Write([]byte(conf.content))
		confs[index].file = file
		defer os.Remove(file.Name())
	}

	// adding temp files into config
	config.SetConfigFile(confs[0].file.Name())
	err := config.AddExtraConfigPaths(func() []string {
		res := []string{}
		for _, e := range confs[1:] {
			res = append(res, e.file.Name())
		}
		return res
	}())
	assert.NoError(t, err)

	// loading config files
	err = config.ReadInConfig()
	assert.NoError(t, err)

	assert.Equal(t, nil, config.Get("api_key"))
	assert.Equal(t, "datadoghq.eu", config.Get("site"))
	assert.Equal(t, "https:proxyserver1", config.Get("proxy.https"))
	assert.Equal(t, "http:proxyserver2", config.Get("proxy.http"))
	assert.Equal(t, model.SourceFile, config.GetSource("proxy.https"))

	// Consistency check on ReadInConfig() call

	oldConf := config.AllSettings()

	// reloading config files
	err = config.ReadInConfig()
	assert.NoError(t, err)
	assert.True(t, reflect.DeepEqual(oldConf, config.AllSettings()))
}

func TestMergeFleetPolicy(t *testing.T) {
	config := NewViperConfig("test", "DD", strings.NewReplacer(".", "_")) // nolint: forbidigo
	config.SetConfigType("yaml")
	config.Set("foo", "bar", model.SourceFile)

	file, err := os.CreateTemp("", "datadog.yaml")
	assert.NoError(t, err, "failed to create temporary file: %w", err)
	file.Write([]byte("foo: baz"))
	err = config.MergeFleetPolicy(file.Name())
	assert.NoError(t, err)

	assert.Equal(t, "baz", config.Get("foo"))
	assert.Equal(t, model.SourceFleetPolicies, config.GetSource("foo"))
}

func TestParseEnvAsStringSlice(t *testing.T) {
	config := NewViperConfig("test", "DD", strings.NewReplacer(".", "_")) // nolint: forbidigo

	config.BindEnv("slice_of_string")
	config.ParseEnvAsStringSlice("slice_of_string", func(string) []string { return []string{"a", "b", "c"} })

	t.Setenv("DD_SLICE_OF_STRING", "__some_data__")
	assert.Equal(t, []string{"a", "b", "c"}, config.Get("slice_of_string"))
}

func TestParseEnvAsMapStringInterface(t *testing.T) {
	config := NewViperConfig("test", "DD", strings.NewReplacer(".", "_")) // nolint: forbidigo

	config.BindEnv("map_of_float")
	config.ParseEnvAsMapStringInterface("map_of_float", func(string) map[string]interface{} { return map[string]interface{}{"a": 1.0, "b": 2.0, "c": 3.0} })

	t.Setenv("DD_MAP_OF_FLOAT", "__some_data__")
	assert.Equal(t, map[string]interface{}{"a": 1.0, "b": 2.0, "c": 3.0}, config.Get("map_of_float"))
	assert.Equal(t, map[string]interface{}{"a": 1.0, "b": 2.0, "c": 3.0}, config.GetStringMap("map_of_float"))
}

func TestParseEnvAsSliceMapString(t *testing.T) {
	config := NewViperConfig("test", "DD", strings.NewReplacer(".", "_")) // nolint: forbidigo

	config.BindEnv("map")
	config.ParseEnvAsSliceMapString("map", func(string) []map[string]string { return []map[string]string{{"a": "a", "b": "b", "c": "c"}} })

	t.Setenv("DD_MAP", "__some_data__")
	assert.Equal(t, []map[string]string{{"a": "a", "b": "b", "c": "c"}}, config.Get("map"))
}

func TestListenersUnsetForSource(t *testing.T) {
	config := NewViperConfig("test", "DD", strings.NewReplacer(".", "_")) // nolint: forbidigo

	// Create a listener that will keep track of the changes
	logLevels := []string{}
	notifications := make(chan string, 10)

	config.OnUpdate(func(_ string, _, next any, _ uint64) {
		nextString := next.(string)
		notifications <- nextString
	})

	config.Set("log_level", "info", model.SourceFile)
	config.Set("log_level", "debug", model.SourceRC)
	config.UnsetForSource("log_level", model.SourceRC)
	timeout := time.After(5 * time.Second)

	for len(logLevels) < 3 {
		select {
		case level := <-notifications:
			logLevels = append(logLevels, level)
		case <-timeout:
			t.Fatal("Timeout waiting for notifications")
		}
	}
	assert.Equal(t, []string{"info", "debug", "info"}, logLevels)
}

func TestUnsetForSourceRemoveIfNotPrevious(t *testing.T) {
	cfg := NewViperConfig("test", "TEST", strings.NewReplacer(".", "_"))
	cfg.BindEnv("api_key")
	cfg.BuildSchema()

	// api_key is not in the config (does not have a default value)
	assert.Equal(t, "", cfg.GetString("api_key"))
	_, found := cfg.AllSettings()["api_key"]
	assert.False(t, found)

	cfg.Set("api_key", "0123456789abcdef", model.SourceAgentRuntime)

	// api_key is set
	assert.Equal(t, "0123456789abcdef", cfg.GetString("api_key"))
	_, found = cfg.AllSettings()["api_key"]
	assert.True(t, found)

	cfg.UnsetForSource("api_key", model.SourceAgentRuntime)

	// api_key is unset, which means its not listed in AllSettings
	assert.Equal(t, "", cfg.GetString("api_key"))
	_, found = cfg.AllSettings()["api_key"]
	assert.False(t, found)

	cfg.SetWithoutSource("api_key", "0123456789abcdef")

	// api_key is set
	assert.Equal(t, "0123456789abcdef", cfg.GetString("api_key"))
	_, found = cfg.AllSettings()["api_key"]
	assert.True(t, found)

	cfg.UnsetForSource("api_key", model.SourceUnknown)

	// api_key is unset again, should not appear in AllSettings
	assert.Equal(t, "", cfg.GetString("api_key"))
	_, found = cfg.AllSettings()["api_key"]
	assert.False(t, found)
}

func TestSetWithEnvTransformer(t *testing.T) {
	cfg := NewViperConfig("test", "TEST", strings.NewReplacer(".", "_"))
	cfg.BindEnvAndSetDefault("setting", []string{"default"})
	cfg.ParseEnvAsStringSlice("setting", func(in string) []string {
		return strings.Split(in, ",")
	})
	t.Setenv("TEST_SETTING", "a,b,c,d")
	cfg.BuildSchema()

	assert.Equal(t, []string{"a", "b", "c", "d"}, cfg.GetStringSlice("setting"))

	// setting a value at a lower level of importance should not impact the result of Get
	cfg.Set("setting", []string{"z", "y", "x"}, model.SourceFile)

	assert.Equal(t, []string{"a", "b", "c", "d"}, cfg.GetStringSlice("setting"))

	// setting a value at a higher level of importance
	cfg.Set("setting", []string{"runtime"}, model.SourceAgentRuntime)

	assert.Equal(t, []string{"runtime"}, cfg.GetStringSlice("setting"))
}

func TestSequenceID(t *testing.T) {
	config := NewViperConfig("test", "DD", strings.NewReplacer(".", "_")) // nolint: forbidigo

	assert.Equal(t, uint64(0), config.GetSequenceID())

	config.Set("foo", "bar", model.SourceAgentRuntime)
	assert.Equal(t, uint64(1), config.GetSequenceID())

	config.Set("foo", "baz", model.SourceAgentRuntime)
	assert.Equal(t, uint64(2), config.GetSequenceID())

	// Setting the same value does not update the sequence ID
	config.Set("foo", "baz", model.SourceAgentRuntime)
	assert.Equal(t, uint64(2), config.GetSequenceID())

	// Does not update the sequence ID since the source does not match
	config.UnsetForSource("foo", model.SourceCLI)
	assert.Equal(t, uint64(2), config.GetSequenceID())

	config.UnsetForSource("foo", model.SourceAgentRuntime)
	assert.Equal(t, uint64(3), config.GetSequenceID())
}
