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
)

func constructNtmConfig(content string, setupFunc func(model.Setup)) model.BuildableConfig {
	conf := nodetreemodel.NewNodeTreeConfig("datadog", "DD", strings.NewReplacer(".", "_")) // nolint: forbidigo // legit use case

	if setupFunc != nil {
		setupFunc(conf)
	}

	conf.BuildSchema()

	if len(content) > 0 {
		conf.SetConfigType("yaml")
		conf.ReadConfig(bytes.NewBuffer([]byte(content)))
	} else {
		conf.ReadInConfig()
	}

	return conf
}

func TestSetTree(t *testing.T) {
	// One setting comes from the yaml file
	configData := `network_path:
  collector:
    input_chan_size: 23456
    workers: 8
`
	cfg := constructNtmConfig(configData, func(cfg model.Setup) {
		cfg.BindEnvAndSetDefault("network_path.collector.input_chan_size", 0)
		cfg.BindEnvAndSetDefault("network_path.collector.workers", 0)
	})

	SetTree(cfg, "network_path.collector", map[string]interface{}{
		"input_chan_size": 65432,
		"workers":         16,
	}, model.SourceLocalConfigProcess)

	assert.Equal(t, 65432, cfg.GetInt("network_path.collector.input_chan_size"))
	assert.Equal(t, 16, cfg.GetInt("network_path.collector.workers"))
}
