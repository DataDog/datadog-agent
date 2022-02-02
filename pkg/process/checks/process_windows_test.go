// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows
// +build windows

package checks

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
)

func TestPerfCountersConfigSetting(t *testing.T) {
	resetOnce := func() {
		processProbeOnce = sync.Once{}
	}

	t.Run("use toolhelp API", func(t *testing.T) {
		resetOnce()
		defer resetOnce()

		cfg := config.Mock()
		cfg.Set("process_config.windows.use_perf_counters", false)
		probe := getProcessProbe()
		assert.IsType(t, procutil.NewWindowsToolhelpProbe(), probe)
	})

	t.Run("use PDH api", func(t *testing.T) {
		resetOnce()
		defer resetOnce()

		cfg := config.Mock()
		cfg.Set("process_config.windows.use_perf_counters", true)
		probe := getProcessProbe()
		assert.IsType(t, procutil.NewProcessProbe(), probe)
	})
}
