// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build test

package logsagentexporter

import (
	"context"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/exporter/exportertest"
)

func TestNewFactory(t *testing.T) {
	channel := make(chan *message.Message)

	factory := NewFactory(channel)
	cfg := factory.CreateDefaultConfig()
	assert.NoError(t, componenttest.CheckConfigStruct(cfg))
	_, ok := factory.CreateDefaultConfig().(*exporterConfig)
	assert.True(t, ok)
}

func TestNewLogsExporter(t *testing.T) {
	channel := make(chan *message.Message)

	factory := NewFactory(channel)
	cfg := factory.CreateDefaultConfig()

	set := exportertest.NewNopCreateSettings()
	_, err := factory.CreateLogsExporter(context.Background(), set, cfg)
	assert.NoError(t, err)
}
