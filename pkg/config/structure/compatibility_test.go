// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package structure

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/nodetreemodel"
	"github.com/DataDog/datadog-agent/pkg/config/viperconfig"
)

type IntPermutation struct {
	ToInt   int   `mapstructure:"to_int"`
	ToInt32 int32 `mapstructure:"to_int32"`
	ToInt64 int64 `mapstructure:"to_int64"`
}

func constructBothConfigs(content string, dynamicSchema bool, setupFunc func(model.Setup)) (model.Config, model.Config) {
	viperConf := viperconfig.NewViperConfig("datadog", "DD", strings.NewReplacer(".", "_"))    // nolint: forbidigo // legit use case
	ntmConf := nodetreemodel.NewNodeTreeConfig("datadog", "DD", strings.NewReplacer(".", "_")) // nolint: forbidigo // legit use case

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

func TestCompareWrongType(t *testing.T) {
	dataYaml := `
network_devices:
  autodiscovery:
    workers:
      - 10
        11
        12
`
	viperConf, ntmConf := constructBothConfigs(dataYaml, false, func(cfg model.Setup) {
		cfg.SetKnown("network_devices.autodiscovery.workers")
		cfg.SetDefault("network_devices.autodiscovery.workers", 5)
	})

	type simpleConfig struct {
		Workers int `mapstructure:"workers"`
	}
	cfg := simpleConfig{}

	// unable to cast []int to int
	num := viperConf.GetInt("network_devices.autodiscovery.workers")
	assert.Equal(t, 0, num)

	// unable to cast []int to int
	num = ntmConf.GetInt("network_devices.autodiscovery.workers")
	assert.Equal(t, 0, num)

	err := viperConf.UnmarshalKey("network_devices.autodiscovery", &cfg)
	assert.NoError(t, err)
	assert.Equal(t, 0, cfg.Workers)

	// NOTE: Behavior difference! UnmarshalKey will fail here because a leaf node will fail to convert
	err = unmarshalKeyReflection(ntmConf, "network_devices.autodiscovery", &cfg)
	assert.Error(t, err)
}

func TestCompareParseError(t *testing.T) {
	dataYaml := `
network_devices:
  autodiscovery:
    - workers: 10
`
	viperConf, ntmConf := constructBothConfigs(dataYaml, false, func(cfg model.Setup) {
		cfg.SetKnown("network_devices.autodiscovery.workers")
	})

	warnings := viperConf.Warnings()
	assert.Equal(t, model.NewWarnings(nil), warnings)
	assert.Equal(t, 0, len(warnings.Errors))

	// NOTE: An additional warning is created here because the config has an error
	warnings = ntmConf.Warnings()
	assert.Equal(t, 1, len(warnings.Errors))

	type simpleConfig struct {
		Workers int `mapstructure:"workers"`
	}
	cfg := simpleConfig{}

	val := viperConf.Get("network_devices.autodiscovery")
	assert.NotNil(t, val)

	val = ntmConf.Get("network_devices.autodiscovery")
	assert.NotNil(t, val)

	err := viperConf.UnmarshalKey("network_devices.autodiscovery", &cfg)
	assert.Error(t, err)
	assert.ErrorContains(t, err, "'' expected a map, got 'slice'")

	// NOTE: Error message differs, but that is an acceptable difference
	err = unmarshalKeyReflection(ntmConf, "network_devices.autodiscovery", &cfg)
	assert.Error(t, err)
	assert.ErrorContains(t, err, "can't GetChild(workers) of a leaf node")
}

type MetadataProviders struct {
	Name     string        `mapstructure:"name"`
	Interval time.Duration `mapstructure:"interval"`
}

func TestCompareTimeDuration(t *testing.T) {
	viperConf, ntmConf := constructBothConfigs("", false, func(cfg model.Setup) {
		cfg.SetDefault("provider.interval", 5*time.Second)
	})
	assert.Equal(t, 5*time.Second, viperConf.GetDuration("provider.interval"))
	assert.Equal(t, 5*time.Second, ntmConf.GetDuration("provider.interval"))

	var mp1 MetadataProviders
	var mp2 MetadataProviders

	err := UnmarshalKey(viperConf, "provider", &mp1)
	assert.NoError(t, err)
	err = unmarshalKeyReflection(ntmConf, "provider", &mp2)
	assert.NoError(t, err)

	assert.Equal(t, 5*time.Second, mp1.Interval)
	assert.Equal(t, 5*time.Second, mp2.Interval)
}

func TestUnmarshalIntPermutations(t *testing.T) {
	viperConf, ntmConf := constructBothConfigs("", false, func(cfg model.Setup) {
		// All source values set with a mismatched integer type
		cfg.SetDefault("ints.to_int", int64(10))   // int64 → int
		cfg.SetDefault("ints.to_int32", int(20))   // int → int32
		cfg.SetDefault("ints.to_int64", int32(30)) // int32 → int64

		cfg.SetDefault("ints2.to_int", int32(40))   // int32 → int
		cfg.SetDefault("ints2.to_int32", int64(50)) // int64 → int32
		cfg.SetDefault("ints2.to_int64", int(60))   // int → int64
	})

	var (
		viper1, ntm1 IntPermutation
		viper2, ntm2 IntPermutation
	)

	err := viperConf.UnmarshalKey("ints", &viper1)
	assert.NoError(t, err)
	err = unmarshalKeyReflection(ntmConf, "ints", &ntm1)
	assert.NoError(t, err)

	err = viperConf.UnmarshalKey("ints2", &viper2)
	assert.NoError(t, err)
	err = unmarshalKeyReflection(ntmConf, "ints2", &ntm2)
	assert.NoError(t, err)

	assert.Equal(t, int(10), viper1.ToInt)
	assert.Equal(t, int(10), ntm1.ToInt)

	assert.Equal(t, int32(20), viper1.ToInt32)
	assert.Equal(t, int32(20), ntm1.ToInt32)

	assert.Equal(t, int64(30), viper1.ToInt64)
	assert.Equal(t, int64(30), ntm1.ToInt64)

	assert.Equal(t, int(40), viper2.ToInt)
	assert.Equal(t, int(40), ntm2.ToInt)

	assert.Equal(t, int32(50), viper2.ToInt32)
	assert.Equal(t, int32(50), ntm2.ToInt32)

	assert.Equal(t, int64(60), viper2.ToInt64)
	assert.Equal(t, int64(60), ntm2.ToInt64)

	assert.Equal(t, IntPermutation{ToInt: 10, ToInt32: 20, ToInt64: 30}, viper1)
	assert.Equal(t, IntPermutation{ToInt: 10, ToInt32: 20, ToInt64: 30}, ntm1)

	assert.Equal(t, IntPermutation{ToInt: 40, ToInt32: 50, ToInt64: 60}, viper2)
	assert.Equal(t, IntPermutation{ToInt: 40, ToInt32: 50, ToInt64: 60}, ntm2)
}
