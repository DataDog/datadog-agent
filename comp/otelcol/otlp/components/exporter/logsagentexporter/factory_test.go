// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build test

package logsagentexporter

import (
	"context"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/message"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/exporter/exportertest"
)

func TestNewLogsExporter(t *testing.T) {
	channel := make(chan *message.Message)

	factory := NewFactory(channel)
	cfg := factory.CreateDefaultConfig()

	set := exportertest.NewNopCreateSettings()
	_, err := factory.CreateLogsExporter(context.Background(), set, cfg)
	assert.NoError(t, err)
}
