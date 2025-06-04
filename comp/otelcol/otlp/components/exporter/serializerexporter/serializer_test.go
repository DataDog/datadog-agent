// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package serializerexporter

import (
	"context"
	"testing"

	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestInitSerializer(t *testing.T) {
	logger := zap.NewNop()
	var sourceProvider SourceProviderFunc = func(_ context.Context) (string, error) {
		return "test", nil
	}
	cfg := &ExporterConfig{}
	s, fw, err := initSerializer(logger, cfg, sourceProvider)
	assert.Nil(t, err)
	assert.IsType(t, &defaultforwarder.DefaultForwarder{}, fw)
	assert.NotNil(t, fw)
	assert.NotNil(t, s)
}
