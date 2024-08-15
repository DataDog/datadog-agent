// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package ebpf

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/network/config"
)

func TestMaxPostgresTelemetryDefaultConfig(t *testing.T) {
	// Validating that the default value matches the one in the Postgres ebpf package.
	t.Run("default", func(t *testing.T) {
		cfg := config.New()
		assert.Equal(t, BufferSize, cfg.MaxPostgresTelemetryBuffer)
	})
}
