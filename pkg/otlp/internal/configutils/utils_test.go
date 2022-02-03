// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package configutils

import (
	"context"
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/exporter/otlpexporter"
	"go.opentelemetry.io/collector/receiver/otlpreceiver"
	"go.opentelemetry.io/collector/service"
)

const testPath = "./testdata/pipeline.yaml"

func buildTestFactories(t *testing.T) component.Factories {
	extensions, err := component.MakeExtensionFactoryMap()
	require.NoError(t, err)
	processors, err := component.MakeProcessorFactoryMap()
	require.NoError(t, err)
	exporters, err := component.MakeExporterFactoryMap(otlpexporter.NewFactory())
	require.NoError(t, err)
	receivers, err := component.MakeReceiverFactoryMap(otlpreceiver.NewFactory())
	require.NoError(t, err)

	return component.Factories{
		Extensions: extensions,
		Receivers:  receivers,
		Processors: processors,
		Exporters:  exporters,
	}
}

func TestNewConfigProviderFromMap(t *testing.T) {
	// build constant provider
	content, err := ioutil.ReadFile(testPath)
	require.NoError(t, err)
	cfgMap, err := NewMapFromYAMLString(string(content))
	require.NoError(t, err)
	mapProvider := NewConfigProviderFromMap(cfgMap)

	// build default provider from same data
	defaultProvider := service.NewDefaultConfigProvider(
		[]string{fmt.Sprintf("file:%s", testPath)},
		[]string{},
	)

	// Get config.Config from both
	factories := buildTestFactories(t)
	cfg, err := mapProvider.Get(context.Background(), factories)
	require.NoError(t, err)
	defaultCfg, err := defaultProvider.Get(context.Background(), factories)
	require.NoError(t, err)

	assert.Equal(t, cfg, defaultCfg, "Custom constant provider does not provide same config as default provider.")
}
