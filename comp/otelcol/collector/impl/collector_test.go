// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build otlp

// Package collectorimpl provides the implementation of the collector component for OTel Agent
package collectorimpl

import (
	"path/filepath"
	"testing"

	compdef "github.com/DataDog/datadog-agent/comp/def"
	collectorcontribimpl "github.com/DataDog/datadog-agent/comp/otelcol/collector-contrib/impl"
	converterimpl "github.com/DataDog/datadog-agent/comp/otelcol/converter/impl"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/confmap"
)

type lifecycle struct{}

func (l *lifecycle) Append(h compdef.Hook) {
	return
}

func uriFromFile(filename string) []string {
	return []string{filepath.Join("testdata", filename)}
}

func TestGetConfDump(t *testing.T) {
	provider, err := converterimpl.NewConverter()
	assert.NoError(t, err)

	reqs := Requires{
		CollectorContrib: collectorcontribimpl.NewComponent(),
		URIs:             uriFromFile("simple-dd/config.yaml"),
		Provider:         provider,
		Lc:               &lifecycle{},
	}

	provides, err := NewComponent(reqs)
	assert.NoError(t, err)

	providedConf, _ := provides.Comp.GetProvidedConf()
	enhancedConf, _ := provides.Comp.GetEnhancedConf()

	reqsProvided := Requires{
		CollectorContrib: collectorcontribimpl.NewComponent(),
		URIs:             uriFromFile("simple-dd/config-provided-result.yaml"),
		Provider:         provider,
		Lc:               &lifecycle{},
	}
	configResultProvided, err := getConfig(reqsProvided, false)
	assert.NoError(t, err)
	confMapResultProvided := confmap.New()
	err = confMapResultProvided.Marshal(configResultProvided)
	assert.NoError(t, err)
	assert.Equal(t, confMapResultProvided.ToStringMap(), providedConf.ToStringMap())

	reqsEnhanced := Requires{
		CollectorContrib: collectorcontribimpl.NewComponent(),
		URIs:             uriFromFile("simple-dd/config-enhanced-result.yaml"),
		Provider:         provider,
		Lc:               &lifecycle{},
	}
	configResultEnhanced, err := getConfig(reqsEnhanced, true)
	assert.NoError(t, err)
	confMapResultEnhanced := confmap.New()
	err = confMapResultEnhanced.Marshal(configResultEnhanced)
	assert.NoError(t, err)
	assert.Equal(t, confMapResultEnhanced.ToStringMap(), enhancedConf.ToStringMap())
}
