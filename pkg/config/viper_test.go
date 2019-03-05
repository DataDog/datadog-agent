// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package config

import (
	"sync"
	"testing"

	"github.com/DataDog/viper"
	"github.com/stretchr/testify/assert"
)

func TestConcurrencySetGet(t *testing.T) {
	config := safeConfig{
		Viper: viper.New(),
	}

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
	config := safeConfig{
		Viper: viper.New(),
	}
	config.SetDefault("foo", map[string]string{})
	config.SetDefault("BAR", "test")
	config.SetDefault("baz", "test")

	var wg sync.WaitGroup

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
				t.Fatalf("unable to decode into struct, %v", err)
			}
		}
	}()

	wg.Wait()
}

func TestGetConfigEnvVars(t *testing.T) {
	config := safeConfig{
		Viper: viper.New(),
	}
	config.SetEnvPrefix("DD")

	config.BindEnv("app_key")
	assert.Contains(t, config.GetEnvVars(), "DD_APP_KEY")
	config.BindEnv("logset")
	assert.Contains(t, config.GetEnvVars(), "DD_LOGSET")
	config.BindEnv("logs_config.run_path")
	assert.Contains(t, config.GetEnvVars(), "DD_LOGS_CONFIG.RUN_PATH")

	// FIXME: ideally we should also track env vars when BindEnv is used with
	// 2 arguments. Not the case at the moment, as demonstrated below.
	config.BindEnv("config_option", "DD_CONFIG_OPTION")
	assert.NotContains(t, config.GetEnvVars(), "DD_CONFIG_OPTION")
}
