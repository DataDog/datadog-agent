// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build otlp
// +build otlp

package config

import (
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/stretchr/testify/assert"
)

func TestFullYamlConfigWithOTLP(t *testing.T) {
	defer cleanConfig()()
	origcfg := config.Datadog
	config.Datadog = config.NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))
	defer func() {
		config.Datadog = origcfg
	}()

	assert := assert.New(t)

	c, err := prepareConfig("./testdata/full.yaml")
	assert.NoError(err)
	assert.NoError(applyDatadogConfig(c))

	assert.Equal("0.0.0.0", c.OTLPReceiver.BindHost)
	assert.Equal(0, c.OTLPReceiver.HTTPPort)
	assert.Equal(50053, c.OTLPReceiver.GRPCPort)
}
