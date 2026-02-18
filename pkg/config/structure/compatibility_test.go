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
	"github.com/DataDog/datadog-agent/pkg/config/viperconfig"
)

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
	viperConf, ntmConf := constructBothConfigs(dataYaml, false, func(cfg model.Setup) {
		cfg.SetKnown("network_devices.autodiscovery.workers") //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
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

	// same behavior using reflection based UnmarshalKey
	err := UnmarshalKey(viperConf, "network_devices.autodiscovery", &cfg)
	assert.NoError(t, err)
	assert.Equal(t, 0, cfg.Workers)

	// NOTE: Behavior difference! UnmarshalKey will fail here because a leaf node will fail to convert
	err = UnmarshalKey(ntmConf, "network_devices.autodiscovery", &cfg)
	assert.Error(t, err)
}

func TestCompareParseError(t *testing.T) {
	dataYaml := `
network_devices:
  autodiscovery:
    - workers: 10
`
	viperConf, ntmConf := constructBothConfigs(dataYaml, false, func(cfg model.Setup) {
		cfg.SetKnown("network_devices.autodiscovery.workers") //nolint:forbidigo // TODO: replace by 'SetDefaultAndBindEnv'
	})

	warnings := viperConf.Warnings()
	assert.Equal(t, model.NewWarnings(nil), warnings)
	assert.Equal(t, 0, len(warnings.Errors))

	// NOTE: Keys are declared using SetKnown, no warnings generated
	warnings = ntmConf.Warnings()
	assert.Equal(t, 0, len(warnings.Errors))

	type simpleConfig struct {
		Workers int `mapstructure:"workers"`
	}
	cfg := simpleConfig{}

	val := viperConf.Get("network_devices.autodiscovery")
	assert.NotNil(t, val)

	val = ntmConf.Get("network_devices.autodiscovery")
	assert.NotNil(t, val)

	err := UnmarshalKey(viperConf, "network_devices.autodiscovery", &cfg)
	assert.Error(t, err)
	assert.ErrorContains(t, err, "'' expected a map or struct, got \"slice\"")

	err = UnmarshalKey(ntmConf, "network_devices.autodiscovery", &cfg)
	assert.Error(t, err)
	assert.ErrorContains(t, err, "'' expected a map or struct, got \"slice\"")
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
	err = UnmarshalKey(ntmConf, "provider", &mp2)
	assert.NoError(t, err)

	assert.Equal(t, 5*time.Second, mp1.Interval)
	assert.Equal(t, 5*time.Second, mp2.Interval)
}

type IntPermutation struct {
	ToInt   int   `mapstructure:"to_int"`
	ToInt32 int32 `mapstructure:"to_int32"`
	ToInt64 int64 `mapstructure:"to_int64"`
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

	err := UnmarshalKey(viperConf, "ints", &viper1)
	assert.NoError(t, err)
	err = UnmarshalKey(ntmConf, "ints", &ntm1)
	assert.NoError(t, err)

	err = UnmarshalKey(viperConf, "ints2", &viper2)
	assert.NoError(t, err)
	err = UnmarshalKey(ntmConf, "ints2", &ntm2)
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

type MyFeature struct {
	Name    string
	Workers uint32
	Small   int8
	Count   uint8
	Large   uint64
}

func TestCompareNegativeNumber(t *testing.T) {
	viperConf, ntmConf := constructBothConfigs("", false, func(cfg model.Setup) {
		cfg.SetDefault("my_feature.name", "foo")
		cfg.SetDefault("my_feature.workers", -5)
		cfg.SetDefault("my_feature.small", -777)
		cfg.SetDefault("my_feature.count", -777)
		cfg.SetDefault("my_feature.large", -777)
	})

	var mf1 MyFeature
	var mf2 MyFeature

	err := UnmarshalKey(viperConf, "my_feature", &mf1)
	assert.NoError(t, err)
	err = UnmarshalKey(ntmConf, "my_feature", &mf2)
	assert.NoError(t, err)

	assert.Equal(t, "foo", mf1.Name)
	assert.Equal(t, "foo", mf2.Name)

	assert.Equal(t, uint32(0xfffffffb), mf1.Workers)
	assert.Equal(t, uint32(0xfffffffb), mf2.Workers)

	assert.Equal(t, int8(-9), mf1.Small) // 777 % 256 = 9
	assert.Equal(t, int8(-9), mf2.Small)

	assert.Equal(t, uint8(0xf7), mf1.Count)
	assert.Equal(t, uint8(0xf7), mf2.Count)

	assert.Equal(t, uint64(0xfffffffffffffcf7), mf1.Large)
	assert.Equal(t, uint64(0xfffffffffffffcf7), mf2.Large)
}

func TestUnmarshalKeyMapToBools(t *testing.T) {
	viperConf, ntmConf := constructBothConfigs("", false, func(cfg model.Setup) {
		cfg.BindEnvAndSetDefault("test", map[string]bool{})
	})

	type testBool struct {
		A bool
		B bool
	}

	objBool1 := testBool{}
	objBool2 := testBool{}

	viperConf.Set("test", map[string]bool{"a": false, "b": true}, model.SourceAgentRuntime)
	err := UnmarshalKey(viperConf, "test", &objBool1)
	require.NoError(t, err)
	assert.Equal(t, objBool1, testBool{A: false, B: true})

	ntmConf.Set("test", map[string]bool{"a": false, "b": true}, model.SourceAgentRuntime)
	err = UnmarshalKey(ntmConf, "test", &objBool2)
	require.NoError(t, err)
	assert.Equal(t, objBool2, testBool{A: false, B: true})
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
	viperConf, ntmConf := constructBothConfigs(dataYaml, false, func(cfg model.Setup) {
		cfg.SetKnown("my_target") //nolint:forbidigo // legit usage, often used for UnmarshalKey settings
	})

	var target TargetStruct
	err := UnmarshalKey(viperConf, "my_target", &target)
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

	target = TargetStruct{}
	err = UnmarshalKey(ntmConf, "my_target", &target)
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
