// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package nodetreemodel defines a model for the config using a tree of nodes
package nodetreemodel

import (
	"bytes"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/viperconfig"
	"github.com/stretchr/testify/assert"
)

func constructBothConfigs(content string, dynamicSchema bool, setupFunc func(model.Setup)) (model.Config, model.Config) {
	viperConf := viperconfig.NewViperConfig("datadog", "DD", strings.NewReplacer(".", "_")) // nolint: forbidigo // legit use case
	ntmConf := NewNodeTreeConfig("datadog", "DD", strings.NewReplacer(".", "_"))            // nolint: forbidigo // legit use case

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

func TestCompareGetInt(t *testing.T) {
	dataYaml := `port: 345`
	viperConf, ntmConf := constructBothConfigs(dataYaml, true, nil)

	assert.Equal(t, 345, viperConf.GetInt("port"))
	assert.Equal(t, 345, ntmConf.GetInt("port"))

	viperConf, ntmConf = constructBothConfigs(dataYaml, false, func(cfg model.Setup) {
		cfg.SetKnown("port")
	})
	assert.Equal(t, 345, viperConf.GetInt("port"))
	assert.Equal(t, 345, ntmConf.GetInt("port"))
}

func TestCompareIsSet(t *testing.T) {
	dataYaml := `port: 345`
	viperConf, ntmConf := constructBothConfigs(dataYaml, true, nil)
	assert.Equal(t, true, viperConf.IsSet("port"))
	assert.Equal(t, true, ntmConf.IsSet("port"))

	viperConf, ntmConf = constructBothConfigs(dataYaml, false, func(cfg model.Setup) {
		cfg.SetKnown("port")
	})
	assert.Equal(t, true, viperConf.IsSet("port"))
	assert.Equal(t, true, ntmConf.IsSet("port"))

	dataYaml = ``
	viperConf, ntmConf = constructBothConfigs(dataYaml, false, func(cfg model.Setup) {
		cfg.SetDefault("port", 123)
	})
	assert.Equal(t, 123, viperConf.GetInt("port"))
	assert.Equal(t, 123, ntmConf.GetInt("port"))
	assert.Equal(t, true, viperConf.IsSet("port"))
	assert.Equal(t, true, ntmConf.IsSet("port"))

	t.Setenv("TEST_PORT", "789")
	viperConf, ntmConf = constructBothConfigs(dataYaml, false, func(cfg model.Setup) {
		cfg.BindEnv("port", "TEST_PORT")
	})
	assert.Equal(t, 789, viperConf.GetInt("port"))
	assert.Equal(t, 789, ntmConf.GetInt("port"))
	assert.Equal(t, true, viperConf.IsSet("port"))
	assert.Equal(t, true, ntmConf.IsSet("port"))

	t.Setenv("TEST_PORT", "")
	viperConf, ntmConf = constructBothConfigs(dataYaml, false, func(cfg model.Setup) {
		cfg.BindEnv("port", "TEST_PORT")
	})
	assert.Equal(t, 0, viperConf.GetInt("port"))
	assert.Equal(t, 0, ntmConf.GetInt("port"))
	assert.Equal(t, false, viperConf.IsSet("port"))
	assert.Equal(t, false, ntmConf.IsSet("port"))
}
