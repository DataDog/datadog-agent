// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build test
// +build test

package serializerexporter

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/configtest"

	"github.com/DataDog/datadog-agent/pkg/serializer"
)

func TestNewFactory(t *testing.T) {
	factory := NewFactory(&serializer.MockSerializer{})
	cfg := factory.CreateDefaultConfig()
	assert.NoError(t, configtest.CheckConfigStruct(cfg))
	_, ok := factory.CreateDefaultConfig().(*exporterConfig)
	assert.True(t, ok)
}

func TestNewMetricsExporter(t *testing.T) {
	factory := NewFactory(&serializer.MockSerializer{})
	cfg := factory.CreateDefaultConfig()
	set := componenttest.NewNopExporterCreateSettings()
	exp, err := factory.CreateMetricsExporter(context.Background(), set, cfg)
	assert.NoError(t, err)
	assert.NotNil(t, exp)
}

func TestNewTracesExporter(t *testing.T) {
	factory := NewFactory(&serializer.MockSerializer{})
	cfg := factory.CreateDefaultConfig()

	set := componenttest.NewNopExporterCreateSettings()
	_, err := factory.CreateTracesExporter(context.Background(), set, cfg)
	assert.Error(t, err)
}

func TestNewLogsExporter(t *testing.T) {
	factory := NewFactory(&serializer.MockSerializer{})
	cfg := factory.CreateDefaultConfig()

	set := componenttest.NewNopExporterCreateSettings()
	_, err := factory.CreateLogsExporter(context.Background(), set, cfg)
	assert.Error(t, err)
}
