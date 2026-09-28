// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package defaultforwarderimpl

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/stretchr/testify/require"
)

func TestTLSFallbackConsumerLifecycle(t *testing.T) {
	cfg := config.NewMock(t)
	cfg.Set("min_tls_version", "invalid", model.SourceFile)
	first := NewSharedConnection(logmock.New(t), false, 1, cfg, nil)
	second := NewSharedConnection(logmock.New(t), false, 1, cfg, nil)
	require.Empty(t, cfg.GetConfigFallbacks())
	first.setActive(true)
	second.setActive(true)
	require.Len(t, cfg.GetConfigFallbacks(), 2)
	first.setActive(false)
	require.Len(t, cfg.GetConfigFallbacks(), 1)
	cfg.Set("min_tls_version", "tlsv1.3", model.SourceFile)
	require.Len(t, cfg.GetConfigFallbacks(), 1)
	second.ResetClient()
	require.Empty(t, cfg.GetConfigFallbacks())
	second.setActive(false)
}
