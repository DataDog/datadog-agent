// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

// Package complianceimpl implements the compliance component interface
package complianceimpl

import (
	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/def"
	compliance "github.com/DataDog/datadog-agent/comp/system-probe/compliance/def"
	"github.com/DataDog/datadog-agent/comp/system-probe/types"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
)

// Requires defines the dependencies for the compliance component
type Requires struct {
	CoreConfig  config.Component
	Hostname    hostname.Component
	Log         log.Component
	Statsd      ddgostatsd.ClientInterface
	WMeta       workloadmeta.Component
	Compression logscompression.Component
	FilterStore workloadfilter.Component
}

// Provides defines the output of the compliance component
type Provides struct {
	Comp   compliance.Component
	Module types.ProvidesSystemProbeModule
}

// NewComponent creates a new compliance component
func NewComponent(reqs Requires) (Provides, error) {
	mc := &moduleFactory{
		createFn: func() (types.SystemProbeModule, error) {
			return newComplianceModule(dependencies{
				CoreConfig:  reqs.CoreConfig,
				Hostname:    reqs.Hostname,
				Log:         reqs.Log,
				Statsd:      reqs.Statsd,
				WMeta:       reqs.WMeta,
				Compression: reqs.Compression,
				FilterStore: reqs.FilterStore,
			})
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
	return sysconfig.ComplianceModule
}

func (m *moduleFactory) ConfigNamespaces() []string {
	return []string{"compliance_config", "runtime_security_config"}
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
