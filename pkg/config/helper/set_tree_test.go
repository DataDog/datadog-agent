// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package helper

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/nodetreemodel"
	"github.com/DataDog/datadog-agent/pkg/config/viperconfig"
)

func constructBothConfigs(content string, setupFunc func(model.Setup)) (model.BuildableConfig, model.BuildableConfig) {
	viperConf := viperconfig.NewViperConfig("datadog", "DD", strings.NewReplacer(".", "_"))    // nolint: forbidigo // legit use case
	ntmConf := nodetreemodel.NewNodeTreeConfig("datadog", "DD", strings.NewReplacer(".", "_")) // nolint: forbidigo // legit use case

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
	} else {
		viperConf.ReadInConfig()
		ntmConf.ReadInConfig()
	}

	return viperConf, ntmConf
}

func TestSetTree(t *testing.T) {
	// One setting comes from the yaml file
	configData := `network_path:
  collector:
    input_chan_size: 23456
    workers: 8
`
	viperCfg, ntmCfg := constructBothConfigs(configData, func(cfg model.Setup) {
		cfg.BindEnvAndSetDefault("network_path.collector.input_chan_size", 0)
		cfg.BindEnvAndSetDefault("network_path.collector.workers", 0)
	})

	SetTree(viperCfg, "network_path.collector", map[string]interface{}{
		"input_chan_size": 65432,
		"workers":         16,
	}, model.SourceLocalConfigProcess)

	assert.Equal(t, 65432, viperCfg.GetInt("network_path.collector.input_chan_size"))
	assert.Equal(t, 16, viperCfg.GetInt("network_path.collector.workers"))

	SetTree(ntmCfg, "network_path.collector", map[string]interface{}{
		"input_chan_size": 65432,
		"workers":         16,
	}, model.SourceLocalConfigProcess)

	assert.Equal(t, 65432, ntmCfg.GetInt("network_path.collector.input_chan_size"))
	assert.Equal(t, 16, ntmCfg.GetInt("network_path.collector.workers"))
}

// recordingReaderWriter records the value of the most recent Set call
type recordingReaderWriter struct {
	model.ReaderWriter
	lastSetKey   string
	lastSetValue interface{}
}

func (r *recordingReaderWriter) Set(key string, value interface{}, source model.Source) {
	r.lastSetKey = key
	r.lastSetValue = value
	r.ReaderWriter.Set(key, value, source)
}

// TestSetTreeCoercesJSONFloatToDeclaredIntType verifies a JSON-decoded float64 is coerced to the declared int type before Set
func TestSetTreeCoercesJSONFloatToDeclaredIntType(t *testing.T) {
	_, ntmCfg := constructBothConfigs("", func(cfg model.Setup) {
		cfg.BindEnvAndSetDefault("network_path.collector.workers", 0)
	})

	rec := &recordingReaderWriter{ReaderWriter: ntmCfg}

	SetTree(rec, "network_path.collector.workers", float64(16), model.SourceLocalConfigProcess)

	assert.IsType(t, int(0), rec.lastSetValue,
		"SetTree should pre-coerce value to declared default type before Set(); got %T", rec.lastSetValue)
	assert.Equal(t, 16, rec.lastSetValue)
	assert.Equal(t, 16, ntmCfg.GetInt("network_path.collector.workers"))
}

// TestSetTreeCoercesJSONFloatInNestedMap verifies pre-coercion through the nested-map recursion path
func TestSetTreeCoercesJSONFloatInNestedMap(t *testing.T) {
	_, ntmCfg := constructBothConfigs("", func(cfg model.Setup) {
		cfg.BindEnvAndSetDefault("network_path.collector.input_chan_size", 0)
		cfg.BindEnvAndSetDefault("network_path.collector.workers", 0)
	})

	rec := &recordingReaderWriter{ReaderWriter: ntmCfg}

	SetTree(rec, "network_path.collector", map[string]interface{}{
		"input_chan_size": float64(65432),
		"workers":         float64(16),
	}, model.SourceLocalConfigProcess)

	assert.IsType(t, int(0), rec.lastSetValue,
		"SetTree should pre-coerce nested-map leaf values; got %T", rec.lastSetValue)
	assert.Equal(t, 65432, ntmCfg.GetInt("network_path.collector.input_chan_size"))
	assert.Equal(t, 16, ntmCfg.GetInt("network_path.collector.workers"))
}
