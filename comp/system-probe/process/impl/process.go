// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

// Package processimpl implements the process component interface
package processimpl

import (
	"os"
	"path/filepath"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	processdef "github.com/DataDog/datadog-agent/comp/system-probe/process/def"
	"github.com/DataDog/datadog-agent/comp/system-probe/types"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
)

// Requires defines the dependencies for the process component
type Requires struct {
	Log log.Component
}

// Provides defines the output of the process component
type Provides struct {
	Comp   processdef.Component
	Module types.ProvidesSystemProbeModule
}

// NewComponent creates a new process component
func NewComponent(reqs Requires) (Provides, error) {
	mc := &moduleFactory{
		createFn: func() (types.SystemProbeModule, error) {
			reqs.Log.Infof("Creating process module for: %s", filepath.Base(os.Args[0]))

			// we disable returning zero values for stats to reduce parsing work on process-agent side
			p := procutil.NewProcessProbe(procutil.WithReturnZeroPermStats(false))
			return &process{
				probe: p,
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
	return config.ProcessModule
}

func (m *moduleFactory) ConfigNamespaces() []string {
	return nil
}

func (m *moduleFactory) Create() (types.SystemProbeModule, error) {
	return m.createFn()
}

func (m *moduleFactory) NeedsEBPF() bool {
	return false
}

func (m *moduleFactory) OptionalEBPF() bool {
	return false
}
