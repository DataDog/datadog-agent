// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package datadogrumreceiver

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/receiver/receivertest"
)

func TestType(t *testing.T) {
	factory := NewFactory()
	pType := factory.Type()

	assert.Equal(t, pType, Type)
}

func TestCreateDefaultConfig(t *testing.T) {
	factory := NewFactory()
	cfg := factory.CreateDefaultConfig()

	assert.NoError(t, componenttest.CheckConfigStruct(cfg))

	rumCfg := cfg.(*Config)
	assert.Equal(t, "localhost:12722", rumCfg.Endpoint)
	assert.Equal(t, 60*time.Second, rumCfg.ReadTimeout)
}

func TestCreateTracesReceiver(t *testing.T) {
	factory := NewFactory()
	cfg := factory.CreateDefaultConfig()

	set := receivertest.NewNopSettings(Type)
	consumer := consumertest.NewNop()

	receiver, err := factory.CreateTraces(context.Background(), set, cfg, consumer)
	assert.NoError(t, err)
	assert.NotNil(t, receiver)
}

func TestCreateLogsReceiver(t *testing.T) {
	factory := NewFactory()
	cfg := factory.CreateDefaultConfig()

	set := receivertest.NewNopSettings(Type)
	consumer := consumertest.NewNop()

	receiver, err := factory.CreateLogs(context.Background(), set, cfg, consumer)
	assert.NoError(t, err)
	assert.NotNil(t, receiver)
}
