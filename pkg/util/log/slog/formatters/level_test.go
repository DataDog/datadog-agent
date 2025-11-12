// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package formatters

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/util/log/slog/types"
)

func TestLevelToString(t *testing.T) {
	tests := []struct {
		level    slog.Level
		expected string
	}{
		{slog.Level(types.TraceLvl), "trace"},
		{slog.Level(types.DebugLvl), "debug"},
		{slog.Level(types.InfoLvl), "info"},
		{slog.Level(types.WarnLvl), "warn"},
		{slog.Level(types.ErrorLvl), "error"},
		{slog.Level(types.CriticalLvl), "critical"},
		{slog.Level(types.Off), "off"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := LevelToString(tt.level)
			assert.Equal(t, tt.expected, result)
		})
	}
}
