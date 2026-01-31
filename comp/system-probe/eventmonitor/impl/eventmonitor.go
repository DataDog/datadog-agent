// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux || windows

// Package eventmonitorimpl implements the eventmonitor component interface
package eventmonitorimpl

import (
	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/cmd/system-probe/modules"
	"github.com/DataDog/datadog-agent/comp/core/hostname"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/def"
	eventmonitor "github.com/DataDog/datadog-agent/comp/system-probe/eventmonitor/def"
	"github.com/DataDog/datadog-agent/comp/system-probe/module"
	"github.com/DataDog/datadog-agent/comp/system-probe/types"
	sysmodule "github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
)

// Requires defines the dependencies for the eventmonitor component
type Requires struct {
	SysprobeConfig sysprobeconfig.Component
	Statsd         ddgostatsd.ClientInterface
	Tagger         tagger.Component
	Hostname       hostname.Component
	WMeta          workloadmeta.Component
	FilterStore    workloadfilter.Component
	Compression    logscompression.Component
	Ipc            ipc.Component
	Log            log.Component
}

// Provides defines the output of the eventmonitor component
type Provides struct {
	Comp   eventmonitor.Component
	Module types.ProvidesSystemProbeModule
}

// NewComponent creates a new eventmonitor component
func NewComponent(reqs Requires) (Provides, error) {
	mc := &module.Component{
		Factory: modules.EventMonitor,
		CreateFn: func() (types.SystemProbeModule, error) {
			return modules.EventMonitor.Fn(nil, sysmodule.FactoryDependencies{
				SysprobeConfig: reqs.SysprobeConfig,
				Log:            reqs.Log,
				WMeta:          reqs.WMeta,
				FilterStore:    reqs.FilterStore,
				Tagger:         reqs.Tagger,
				Compression:    reqs.Compression,
				Statsd:         reqs.Statsd,
				Hostname:       reqs.Hostname,
				Ipc:            reqs.Ipc,
			})
		},
	}
	provides := Provides{
		Module: types.ProvidesSystemProbeModule{Component: mc},
		Comp:   mc,
	}
	return provides, nil
}
