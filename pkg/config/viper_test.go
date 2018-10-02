// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package config

import (
	"sync"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestConcurrency(t *testing.T) {
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
