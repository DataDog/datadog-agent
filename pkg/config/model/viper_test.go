// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package model

import (
	"bytes"
	"fmt"
	"os"
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
	assert.Nil(t, err)
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
	assert.Nil(t, err)
	assert.Equal(t, []float64{1.1, 2.2, 3.3}, list)
}

func TestIsSectionSet(t *testing.T) {
	config := NewConfig("test", "DD", strings.NewReplacer(".", "_"))

	config.BindEnv("test.key")
	config.BindEnv("othertest.key")
	config.SetKnown("yetanothertest_key")
	config.SetConfigType("yaml")

	yamlExample := []byte(`
test:
  key:
`)

	config.ReadConfig(bytes.NewBuffer(yamlExample))

	res := config.IsSectionSet("test")
	assert.Equal(t, true, res)

	res = config.IsSectionSet("othertest")
	assert.Equal(t, false, res)

	t.Setenv("DD_OTHERTEST_KEY", "value")

	res = config.IsSectionSet("othertest")
	assert.Equal(t, true, res)

	config.SetWithoutSource("yetanothertest_key", "value")
	res = config.IsSectionSet("yetanothertest")
	assert.Equal(t, false, res)
}

func TestSet(t *testing.T) {
	config := NewConfig("test", "DD", strings.NewReplacer(".", "_"))
	config.Set("foo", "bar", SourceFile)
	config.Set("foo", "baz", SourceEnvVar)
	config.Set("foo", "qux", SourceAgentRuntime)
	config.Set("foo", "quux", SourceRC)
	config.Set("foo", "corge", SourceCLI)

	assert.Equal(t, config.AllSourceSettingsWithoutDefault(SourceFile), map[string]interface{}{"foo": "bar"})
	assert.Equal(t, config.AllSourceSettingsWithoutDefault(SourceEnvVar), map[string]interface{}{"foo": "baz"})
	assert.Equal(t, config.AllSourceSettingsWithoutDefault(SourceAgentRuntime), map[string]interface{}{"foo": "qux"})
	assert.Equal(t, config.AllSourceSettingsWithoutDefault(SourceRC), map[string]interface{}{"foo": "quux"})
	assert.Equal(t, config.AllSourceSettingsWithoutDefault(SourceCLI), map[string]interface{}{"foo": "corge"})

	assert.Equal(t, config.Get("foo"), "corge")
}

func TestGetSource(t *testing.T) {
	config := NewConfig("test", "DD", strings.NewReplacer(".", "_"))
	config.Set("foo", "bar", SourceFile)
	config.Set("foo", "baz", SourceEnvVar)
	assert.Equal(t, SourceEnvVar, config.GetSource("foo"))
}

func TestIsSet(t *testing.T) {
	config := NewConfig("test", "DD", strings.NewReplacer(".", "_"))
	assert.False(t, config.IsSetForSource("foo", SourceFile))
	config.Set("foo", "bar", SourceFile)
	assert.True(t, config.IsSetForSource("foo", SourceFile))
}

func TestUnsetForSource(t *testing.T) {
	config := NewConfig("test", "DD", strings.NewReplacer(".", "_"))
	config.Set("foo", "bar", SourceFile)
	config.UnsetForSource("foo", SourceFile)
	assert.False(t, config.IsSetForSource("foo", SourceFile))
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
		config.AllSourceSettingsWithoutDefault(SourceFile),
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
	assert.Equal(t, map[string]interface{}{"foo": "bar"}, config.AllSourceSettingsWithoutDefault(SourceFile))
}

func TestNotification(t *testing.T) {
	config := NewConfig("test", "DD", strings.NewReplacer(".", "_"))

	updatedKeyCB1 := []string{}
	updatedKeyCB2 := []string{}

	config.OnUpdate(func(key string) { updatedKeyCB1 = append(updatedKeyCB1, key) })

	config.Set("foo", "bar", SourceFile)
	assert.Equal(t, []string{"foo"}, updatedKeyCB1)

	config.OnUpdate(func(key string) { updatedKeyCB2 = append(updatedKeyCB2, key) })

	config.Set("foo", "bar2", SourceFile)
	config.Set("foo2", "bar2", SourceFile)
	assert.Equal(t, []string{"foo", "foo", "foo2"}, updatedKeyCB1)
	assert.Equal(t, []string{"foo", "foo2"}, updatedKeyCB2)
}

func TestNotificationCanUseGet(t *testing.T) {
	config := NewConfig("test", "DD", strings.NewReplacer(".", "_"))

	var actualValue interface{}
	config.OnUpdate(func(key string) {
		actualValue = config.Get("foo")
	})

	config.Set("foo", "bar", SourceFile)
	assert.Equal(t, "bar", actualValue)
}
