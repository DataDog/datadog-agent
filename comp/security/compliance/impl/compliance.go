// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package complianceimpl implements the compliance component
package complianceimpl

import (
	"context"

	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	complianceComp "github.com/DataDog/datadog-agent/comp/security/compliance/def"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/def"
	pkgcompliance "github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
)

type complianceAgent struct {
	agent   *pkgcompliance.Agent
	stopper startstop.Stopper
}

// Requires defines the dependencies for the compliance component
type Requires struct {
	compdef.In

	Lc             compdef.Lifecycle
	Log            log.Component
	Config         config.Component
	Statsd         ddgostatsd.ClientInterface
	SysprobeConfig sysprobeconfig.Component
	Wmeta          workloadmeta.Component
	FilterStore    workloadfilter.Component
	Compression    logscompression.Component
	Hostname       hostnameinterface.Component
}

// Provides defines the output of the compliance component
type Provides struct {
	compdef.Out

	Comp           complianceComp.Component
	StatusProvider status.InformationProvider
}

// NewComponent creates a new compliance Component
func NewComponent(deps Requires) (Provides, error) {
	// Check if compliance should run in system-probe instead
	if deps.Config.GetBool("compliance_config.run_in_system_probe") {
		deps.Log.Info("compliance_config.run_in_system_probe is enabled, compliance will run in system-probe")
		return Provides{
			Comp:           nil,
			StatusProvider: status.NewInformationProvider(nil),
		}, nil
	}

	// Check if compliance is enabled
	if !deps.Config.GetBool("compliance_config.enabled") {
		return Provides{
			Comp:           nil,
			StatusProvider: status.NewInformationProvider(nil),
		}, nil
	}

	hostnameDetected, err := deps.Hostname.Get(context.TODO())
	if err != nil {
		return Provides{
			Comp:           nil,
			StatusProvider: status.NewInformationProvider(nil),
		}, err
	}

	var sysProbeClient pkgcompliance.SysProbeClient
	if cfg := deps.SysprobeConfig.SysProbeObject(); cfg != nil && cfg.SocketAddress != "" {
		sysProbeClient = pkgcompliance.NewRemoteSysProbeClient(cfg.SocketAddress)
	}

	stopper := startstop.NewSerialStopper()

	// Start compliance agent
	agent, err := pkgcompliance.StartCompliance(
		deps.Log,
		deps.Config,
		hostnameDetected,
		stopper,
		deps.Statsd,
		deps.Wmeta,
		deps.FilterStore,
		deps.Compression,
		sysProbeClient,
	)
	if err != nil {
		return Provides{
			Comp:           nil,
			StatusProvider: status.NewInformationProvider(nil),
		}, err
	}

	comp := &complianceAgent{
		agent:   agent,
		stopper: stopper,
	}

	deps.Lc.Append(compdef.Hook{
		OnStop: func(_ context.Context) error {
			comp.stopper.Stop()
			return nil
		},
	})

	return Provides{
		Comp:           comp,
		StatusProvider: status.NewInformationProvider(comp.statusProvider()),
	}, nil
}

func (c *complianceAgent) statusProvider() status.Provider {
	if c.agent == nil {
		return nil
	}
	return c.agent.StatusProvider()
}
