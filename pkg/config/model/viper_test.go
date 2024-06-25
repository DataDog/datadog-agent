// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package model

import (
	"bytes"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConcurrencySetGet(t *testing.T) {
	config := NewConfig("test", "DD", strings.NewReplacer(".", "_"))

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

func TestConcurrencyUnmarshalling(t *testing.T) {
	config := NewConfig("test", "DD", strings.NewReplacer(".", "_"))

	config.SetDefault("foo", map[string]string{})
	config.SetDefault("BAR", "test")
	config.SetDefault("baz", "test")

	var wg sync.WaitGroup
	errs := make(chan error, 1000)

	wg.Add(2)
	go func() {
		defer wg.Done()
		for n := 0; n <= 1000; n++ {
			config.GetStringMapString("foo")
		}
	}()

	var s *[]string
	go func() {
		defer wg.Done()
		for n := 0; n <= 1000; n++ {
			err := config.UnmarshalKey("foo", &s)
			if err != nil {
				errs <- fmt.Errorf("unable to decode into struct, %w", err)
				return
			}
		}
	}()

	go func() {
		wg.Wait()
		close(errs)
	}()

	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestGetConfigEnvVars(t *testing.T) {
	config := NewConfig("test", "DD", strings.NewReplacer(".", "_"))

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
	config := NewConfig("test", "DD", strings.NewReplacer(".", "_"))

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

func TestGetFloat64SliceE(t *testing.T) {
	config := NewConfig("test", "DD", strings.NewReplacer(".", "_"))

	config.BindEnv("float_list")
	config.SetConfigType("yaml")
	yamlExample := []byte(`---
float_list:
  - 1.1
  - "2.2"
  - 3.3
`)
	config.ReadConfig(bytes.NewBuffer(yamlExample))

	list, err := config.GetFloat64SliceE("float_list")
	assert.NoError(t, err)
	assert.Equal(t, []float64{1.1, 2.2, 3.3}, list)

	yamlExample = []byte(`---
float_list:
  - a
  - 2.2
  - 3.3
`)
	config.ReadConfig(bytes.NewBuffer(yamlExample))

	list, err = config.GetFloat64SliceE("float_list")
	assert.NotNil(t, err)
	assert.Equal(t, "value 'a' from 'float_list' is not a float64", err.Error())
	assert.Nil(t, list)
}

func TestGetFloat64SliceEEnv(t *testing.T) {
	config := NewConfig("test", "DD", strings.NewReplacer(".", "_"))

	config.BindEnv("float_list")
	config.SetConfigType("yaml")

	yamlExample := []byte(`
float_list:
- 25
`)

	config.ReadConfig(bytes.NewBuffer(yamlExample))

	t.Setenv("DD_FLOAT_LIST", "1.1 2.2 3.3")

	list, err := config.GetFloat64SliceE("float_list")
	assert.NoError(t, err)
	assert.Equal(t, []float64{1.1, 2.2, 3.3}, list)
}

func TestSet(t *testing.T) {
	config := NewConfig("test", "DD", strings.NewReplacer(".", "_"))
	config.Set("foo", "bar", SourceFile)
	config.Set("foo", "baz", SourceEnvVar)
	config.Set("foo", "qux", SourceAgentRuntime)
	config.Set("foo", "quux", SourceRC)
	config.Set("foo", "corge", SourceCLI)

	layers := config.AllSettingsBySource()

	assert.Equal(t, layers[SourceFile], map[string]interface{}{"foo": "bar"})
	assert.Equal(t, layers[SourceEnvVar], map[string]interface{}{"foo": "baz"})
	assert.Equal(t, layers[SourceAgentRuntime], map[string]interface{}{"foo": "qux"})
	assert.Equal(t, layers[SourceRC], map[string]interface{}{"foo": "quux"})
	assert.Equal(t, layers[SourceCLI], map[string]interface{}{"foo": "corge"})

	assert.Equal(t, config.Get("foo"), "corge")
}

func TestGetSource(t *testing.T) {
	config := NewConfig("test", "DD", strings.NewReplacer(".", "_"))
	config.Set("foo", "bar", SourceFile)
	config.Set("foo", "baz", SourceEnvVar)
	assert.Equal(t, SourceEnvVar, config.GetSource("foo"))
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
				config := NewConfig("test", "DD", strings.NewReplacer(".", "_"))

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
	config := NewConfig("test", "DD", strings.NewReplacer(".", "_"))
	config.Set("foo", "bar", SourceFile)
	config.Set("baz", "qux", SourceFile)
	config.UnsetForSource("foo", SourceFile)
	assert.Equal(
		t,
		map[string]interface{}{
			"baz": "qux",
		},
		config.AllSettingsWithoutDefault(),
	)
}

func TestSourceFileReadConfig(t *testing.T) {
	config := NewConfig("test", "DD", strings.NewReplacer(".", "_"))
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
	assert.Equal(t, SourceFile, config.GetSource("foo"))
	assert.Equal(t, map[string]interface{}{"foo": "bar"}, config.AllSettingsWithoutDefault())
}

func TestNotification(t *testing.T) {
	config := NewConfig("test", "DD", strings.NewReplacer(".", "_"))

	updatedKeyCB1 := []string{}
	updatedKeyCB2 := []string{}

	config.OnUpdate(func(key string, _, _ any) { updatedKeyCB1 = append(updatedKeyCB1, key) })

	config.Set("foo", "bar", SourceFile)
	assert.Equal(t, []string{"foo"}, updatedKeyCB1)

	config.OnUpdate(func(key string, _, _ any) { updatedKeyCB2 = append(updatedKeyCB2, key) })

	config.Set("foo", "bar2", SourceFile)
	config.Set("foo2", "bar2", SourceFile)
	assert.Equal(t, []string{"foo", "foo", "foo2"}, updatedKeyCB1)
	assert.Equal(t, []string{"foo", "foo2"}, updatedKeyCB2)
}

func TestNotificationNoChange(t *testing.T) {
	config := NewConfig("test", "DD", strings.NewReplacer(".", "_"))

	updatedKeyCB1 := []string{}

	config.OnUpdate(func(key string, _, _ any) { updatedKeyCB1 = append(updatedKeyCB1, key) })

	config.Set("foo", "bar", SourceFile)
	assert.Equal(t, []string{"foo"}, updatedKeyCB1)

	config.Set("foo", "bar", SourceFile)
	assert.Equal(t, []string{"foo"}, updatedKeyCB1)
}

func TestCheckKnownKey(t *testing.T) {
	config := NewConfig("test", "DD", strings.NewReplacer(".", "_")).(*safeConfig)

	config.SetKnown("foo")
	config.Get("foo")
	assert.Empty(t, config.unknownKeys)

	assert.NotContains(t, config.unknownKeys, "foobar")
	config.Get("foobar")
	assert.Contains(t, config.unknownKeys, "foobar")

	config.Get("foobar")
	assert.Contains(t, config.unknownKeys, "foobar")
}

func TestCopyConfig(t *testing.T) {
	config := NewConfig("test", "DD", strings.NewReplacer(".", "_"))
	config.SetDefault("baz", "qux")
	config.Set("foo", "bar", SourceFile)
	config.BindEnv("xyz", "XXYYZZ")
	config.SetKnown("tyu")
	config.OnUpdate(func(key string, _, _ any) {})

	backup := NewConfig("test", "DD", strings.NewReplacer(".", "_"))
	backup.CopyConfig(config)

	assert.Equal(t, "qux", backup.Get("baz"))
	assert.Equal(t, "bar", backup.Get("foo"))
	t.Setenv("XXYYZZ", "value")
	assert.Equal(t, "value", backup.Get("xyz"))
	assert.True(t, backup.IsKnown("tyu"))
	// can't compare function pointers directly so just check the number of callbacks
	assert.Len(t, backup.(*safeConfig).notificationReceivers, 1, "notification receivers should be copied")
}

func TestExtraConfig(t *testing.T) {
	config := NewConfig("test", "DD", strings.NewReplacer(".", "_"))

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
	assert.Equal(t, SourceFile, config.GetSource("proxy.https"))

	// Consistency check on ReadInConfig() call

	oldConf := config.AllSettings()

	// reloading config files
	err = config.ReadInConfig()
	assert.NoError(t, err)
	assert.True(t, reflect.DeepEqual(oldConf, config.AllSettings()))
}
