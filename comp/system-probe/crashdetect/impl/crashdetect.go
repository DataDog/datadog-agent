// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

// Package crashdetectimpl implements the crashdetect component interface
package crashdetectimpl

import (
	"fmt"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	crashdetect "github.com/DataDog/datadog-agent/comp/system-probe/crashdetect/def"
	"github.com/DataDog/datadog-agent/comp/system-probe/types"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/wincrashdetect/probe"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
)

// Requires defines the dependencies for the crashdetect component
type Requires struct {
	SysprobeConfig sysprobeconfig.Component
	Log            log.Component
}

// Provides defines the output of the crashdetect component
type Provides struct {
	Comp   crashdetect.Component
	Module types.ProvidesSystemProbeModule
}

// NewComponent creates a new crashdetect component
func NewComponent(reqs Requires) (Provides, error) {
	mc := &moduleFactory{
		createFn: func() (types.SystemProbeModule, error) {
			reqs.Log.Infof("Starting the WinCrashProbe probe")
			cp, err := probe.NewWinCrashProbe(reqs.SysprobeConfig.SysProbeObject())
			if err != nil {
				return nil, fmt.Errorf("unable to start the Windows Crash Detection probe: %w", err)
			}
			return &winCrashDetectModule{
				WinCrashProbe: cp,
			}, nil
		},
	}
	provides := Provides{
		Module: types.ProvidesSystemProbeModule{Component: mc},
		Comp:   mc,
	}
	return provides, nil
}

type moduleFactory struct {
	createFn func() (types.SystemProbeModule, error)
}

func (m *moduleFactory) Name() sysconfigtypes.ModuleName {
	return config.WindowsCrashDetectModule
}

func (m *moduleFactory) ConfigNamespaces() []string {
	return []string{"windows_crash_detection"}
}

func (m *moduleFactory) Create() (types.SystemProbeModule, error) {
	return m.createFn()
}
