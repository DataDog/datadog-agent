// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build debug

//nolint:revive // TODO Fix revive linter
package start

import (
	"fmt"
	_ "net/http/pprof"
	"runtime"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/util/profiling"
)

func setupInternalProfiling(config config.Component) error {
	runtime.MemProfileRate = 1
	site := fmt.Sprintf(profiling.ProfilingURLTemplate, config.GetString("site"))

	// We need the trace agent runnning to send profiles
	profSettings := profiling.Settings{
		ProfilingURL:         site,
		Socket:               "/var/run/datadog/apm.socket",
		Env:                  "local",
		Service:              "checks-agent",
		Period:               config.GetDuration("internal_profiling.period"),
		CPUDuration:          config.GetDuration("internal_profiling.cpu_duration"),
		MutexProfileFraction: config.GetInt("internal_profiling.mutex_profile_fraction"),
		BlockProfileRate:     config.GetInt("internal_profiling.block_profile_rate"),
		WithGoroutineProfile: config.GetBool("internal_profiling.enable_goroutine_stacktraces"),
		WithBlockProfile:     config.GetBool("internal_profiling.enable_block_profiling"),
		WithMutexProfile:     config.GetBool("internal_profiling.enable_mutex_profiling"),
		WithDeltaProfiles:    config.GetBool("internal_profiling.delta_profiles"),
		CustomAttributes:     config.GetStringSlice("internal_profiling.custom_attributes"),
	}

	return profiling.Start(profSettings)
}
