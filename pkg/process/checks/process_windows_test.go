// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows
// +build windows

package checks

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config"
)

func TestPerfCountersConfigSetting(t *testing.T) {
	cfg := config.Mock()
	cfg.Set("process_config.windows.use_perf_counters", true)
	probe := getProcessProbe()
	assert.Equal(t, probe, defaultWindowsProbe)
}
