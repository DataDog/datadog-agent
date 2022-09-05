// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"bytes"
	"fmt"
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
			config.Set("foo", "bar")
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

	config.Set("yetanothertest_key", "value")
	res = config.IsSectionSet("yetanothertest")
	assert.Equal(t, false, res)
}
