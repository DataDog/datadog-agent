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
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/nodetreemodel"
)

func constructNtmConfig(content string, dynamicSchema bool, setupFunc func(model.Setup)) model.Config {
	conf := nodetreemodel.NewNodeTreeConfig("datadog", "DD", strings.NewReplacer(".", "_")) // nolint: forbidigo // legit use case

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
	}

	return conf
}

func TestWrongType(t *testing.T) {
	// NOTE: network_devices.autodiscovery.works should be an int, this config
	// file contains the wrong type
	dataYaml := `
network_devices:
  autodiscovery:
    workers:
      - 10
        11
        12
`
	conf := constructNtmConfig(dataYaml, false, func(cfg model.Setup) {
		cfg.SetDefault("network_devices.autodiscovery.workers", 5)
	})

	type simpleConfig struct {
		Workers int `mapstructure:"workers"`
	}
	cfg := simpleConfig{}

	// unable to cast []int to int
	num := conf.GetInt("network_devices.autodiscovery.workers")
	assert.Equal(t, 0, num)

	// UnmarshalKey fails here because a leaf node fails to convert
	err := UnmarshalKey(conf, "network_devices.autodiscovery", &cfg)
	assert.Error(t, err)
}

func TestParseError(t *testing.T) {
	dataYaml := `
network_devices:
  autodiscovery:
    - workers: 10
`
	conf := constructNtmConfig(dataYaml, false, func(cfg model.Setup) {
		cfg.BindEnvAndSetDefault("network_devices.autodiscovery", []interface{}{})
	})

	// Keys are declared with a default, no warnings generated
	warnings := conf.Warnings()
	assert.Equal(t, 0, len(warnings.Errors))

	type simpleConfig struct {
		Workers int `mapstructure:"workers"`
	}
	cfg := simpleConfig{}

	val := conf.Get("network_devices.autodiscovery")
	assert.NotNil(t, val)

	err := UnmarshalKey(conf, "network_devices.autodiscovery", &cfg)
	assert.Error(t, err)
	assert.ErrorContains(t, err, "'' expected a map or struct, got \"slice\"")
}

type MetadataProviders struct {
	Name     string        `mapstructure:"name"`
	Interval time.Duration `mapstructure:"interval"`
}

func TestTimeDuration(t *testing.T) {
	conf := constructNtmConfig("", false, func(cfg model.Setup) {
		cfg.SetDefault("provider.interval", 5*time.Second)
	})
	assert.Equal(t, 5*time.Second, conf.GetDuration("provider.interval"))

	var mp MetadataProviders

	err := UnmarshalKey(conf, "provider", &mp)
	assert.NoError(t, err)

	assert.Equal(t, 5*time.Second, mp.Interval)
}

type IntPermutation struct {
	ToInt   int   `mapstructure:"to_int"`
	ToInt32 int32 `mapstructure:"to_int32"`
	ToInt64 int64 `mapstructure:"to_int64"`
}

func TestUnmarshalIntPermutations(t *testing.T) {
	conf := constructNtmConfig("", false, func(cfg model.Setup) {
		// All source values set with a mismatched integer type
		cfg.SetDefault("ints.to_int", int64(10))   // int64 → int
		cfg.SetDefault("ints.to_int32", int(20))   // int → int32
		cfg.SetDefault("ints.to_int64", int32(30)) // int32 → int64

		cfg.SetDefault("ints2.to_int", int32(40))   // int32 → int
		cfg.SetDefault("ints2.to_int32", int64(50)) // int64 → int32
		cfg.SetDefault("ints2.to_int64", int(60))   // int → int64
	})

	var ntm1, ntm2 IntPermutation

	err := UnmarshalKey(conf, "ints", &ntm1)
	assert.NoError(t, err)

	err = UnmarshalKey(conf, "ints2", &ntm2)
	assert.NoError(t, err)

	assert.Equal(t, int(10), ntm1.ToInt)
	assert.Equal(t, int32(20), ntm1.ToInt32)
	assert.Equal(t, int64(30), ntm1.ToInt64)

	assert.Equal(t, int(40), ntm2.ToInt)
	assert.Equal(t, int32(50), ntm2.ToInt32)
	assert.Equal(t, int64(60), ntm2.ToInt64)

	assert.Equal(t, IntPermutation{ToInt: 10, ToInt32: 20, ToInt64: 30}, ntm1)
	assert.Equal(t, IntPermutation{ToInt: 40, ToInt32: 50, ToInt64: 60}, ntm2)
}

type MyFeature struct {
	Name    string
	Workers uint32
	Small   int8
	Count   uint8
	Large   uint64
}

func TestNegativeNumber(t *testing.T) {
	conf := constructNtmConfig("", false, func(cfg model.Setup) {
		cfg.SetDefault("my_feature.name", "foo")
		cfg.SetDefault("my_feature.workers", -5)
		cfg.SetDefault("my_feature.small", -777)
		cfg.SetDefault("my_feature.count", -777)
		cfg.SetDefault("my_feature.large", -777)
	})

	var mf MyFeature

	err := UnmarshalKey(conf, "my_feature", &mf)
	assert.NoError(t, err)

	assert.Equal(t, "foo", mf.Name)
	assert.Equal(t, uint32(0xfffffffb), mf.Workers)
	assert.Equal(t, int8(-9), mf.Small) // 777 % 256 = 9
	assert.Equal(t, uint8(0xf7), mf.Count)
	assert.Equal(t, uint64(0xfffffffffffffcf7), mf.Large)
}

func TestUnmarshalKeyMapToBools(t *testing.T) {
	conf := constructNtmConfig("", false, func(cfg model.Setup) {
		cfg.BindEnvAndSetDefault("test", map[string]bool{})
	})

	type testBool struct {
		A bool
		B bool
	}

	objBool := testBool{}

	conf.Set("test", map[string]bool{"a": false, "b": true}, model.SourceAgentRuntime)
	err := UnmarshalKey(conf, "test", &objBool)
	require.NoError(t, err)
	assert.Equal(t, objBool, testBool{A: false, B: true})
}

type TargetStruct struct {
	BoolToString  string   `mapstructure:"bool_to_string"`
	NumToString   string   `mapstructure:"num_to_string"`
	BoolToInt     int      `mapstructure:"bool_to_int"`
	StringToInt   int      `mapstructure:"string_to_int"`
	StringToInt2  int      `mapstructure:"string_to_int2"`
	IntToBool     bool     `mapstructure:"int_to_bool"`
	IntToBool2    bool     `mapstructure:"int_to_bool2"`
	StringToBool  bool     `mapstructure:"string_to_bool"`
	StringToBool2 bool     `mapstructure:"string_to_bool2"`
	StringToBool3 bool     `mapstructure:"string_to_bool3"`
	StringToBool4 bool     `mapstructure:"string_to_bool4"`
	StrSlice      []string `mapstructure:"str_slice"`
	IntSlice      []int    `mapstructure:"int_slice"`
}

func TestUnmarshalWeaklyTyped(t *testing.T) {
	// Validate that UnmarshalKey uses mapstructure's implicit conversion from
	// mapstructure's WeaklyTypedInput setting.
	// https://github.com/go-viper/mapstructure/blob/v2.4.0/mapstructure.go#L257
	dataYaml := `
my_target:
  bool_to_string:  true
  num_to_string:   5
  bool_to_int:     true
  string_to_int:   "345"
  string_to_int2:  "0x123"
  int_to_bool:     3
  int_to_bool2:    0
  string_to_bool:  T
  string_to_bool2: True
  string_to_bool3: "1"
  string_to_bool4: "FALSE"
  str_slice:       abc
  int_slice:       7
`
	conf := constructNtmConfig(dataYaml, false, func(cfg model.Setup) {
		cfg.BindEnvAndSetDefault("my_target", map[string]interface{}{})
	})

	var target TargetStruct
	err := UnmarshalKey(conf, "my_target", &target)
	assert.Equal(t, "1", target.BoolToString)
	assert.Equal(t, "5", target.NumToString)
	assert.Equal(t, 1, target.BoolToInt)
	assert.Equal(t, 345, target.StringToInt)
	assert.Equal(t, 291, target.StringToInt2)
	assert.Equal(t, true, target.IntToBool)
	assert.Equal(t, false, target.IntToBool2)
	assert.Equal(t, true, target.StringToBool)
	assert.Equal(t, true, target.StringToBool2)
	assert.Equal(t, true, target.StringToBool3)
	assert.Equal(t, false, target.StringToBool4)
	assert.Equal(t, []string{"abc"}, target.StrSlice)
	assert.Equal(t, []int{7}, target.IntSlice)
	require.NoError(t, err)
}
