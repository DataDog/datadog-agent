// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package runtimesettingsimpl provide implementation for the runtimesettings.Component
package runtimesettingsimpl

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/agent/runtimesettings"
	settings "github.com/DataDog/datadog-agent/comp/agent/runtimesettings/runtimesettingsimpl/internal"
	"github.com/DataDog/datadog-agent/comp/core/log"
	dogstatsddebug "github.com/DataDog/datadog-agent/comp/dogstatsd/serverDebug"
	commonsettings "github.com/DataDog/datadog-agent/pkg/config/settings"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newRuntimeSettings),
	)
}

type runtime struct{}

type dependencies struct {
	fx.In

	ServerDebug dogstatsddebug.Component
	Logger      log.Component
}

func newRuntimeSettings(deps dependencies) (_ runtimesettings.Component, err error) {
	// Warn error if returned error is not nil
	defer func() {
		if err != nil {
			deps.Logger.Warnf("Can't initiliaze the runtime settings: %v", err)
		}
		err = nil
	}()

	// Runtime-editable settings must be registered here to dynamically populate command-line information
	if err = commonsettings.RegisterRuntimeSetting(commonsettings.NewLogLevelRuntimeSetting()); err != nil {
		return nil, err
	}
	if err = commonsettings.RegisterRuntimeSetting(commonsettings.NewRuntimeMutexProfileFraction()); err != nil {
		return nil, err
	}
	if err = commonsettings.RegisterRuntimeSetting(commonsettings.NewRuntimeBlockProfileRate()); err != nil {
		return nil, err
	}
	if err = commonsettings.RegisterRuntimeSetting(settings.NewDsdStatsRuntimeSetting(deps.ServerDebug)); err != nil {
		return nil, err
	}
	if err = commonsettings.RegisterRuntimeSetting(settings.NewDsdCaptureDurationRuntimeSetting("dogstatsd_capture_duration")); err != nil {
		return nil, err
	}
	if err = commonsettings.RegisterRuntimeSetting(commonsettings.NewLogPayloadsRuntimeSetting()); err != nil {
		return nil, err
	}
	if err = commonsettings.RegisterRuntimeSetting(commonsettings.NewProfilingGoroutines()); err != nil {
		return nil, err
	}
	if err = commonsettings.RegisterRuntimeSetting(
		settings.NewHighAvailabilityRuntimeSetting("ha.enabled", "Enable/disable High Availability support."),
	); err != nil {
		return nil, err
	}
	if err = commonsettings.RegisterRuntimeSetting(
		settings.NewHighAvailabilityRuntimeSetting("ha.failover", "Enable/disable redirection of telemetry data to failover region."),
	); err != nil {
		return nil, err
	}
	if err = commonsettings.RegisterRuntimeSetting(
		commonsettings.NewProfilingRuntimeSetting("internal_profiling", "datadog-agent"),
	); err != nil {
		return nil, err
	}

	return runtime{}, nil
}
