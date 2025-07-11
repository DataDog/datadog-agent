// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package structure

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

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
	assert.Nil(t, warnings)

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
