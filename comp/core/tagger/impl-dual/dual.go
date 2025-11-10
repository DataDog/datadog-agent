// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package dualimpl contains the implementation of the dual tagger.
// The dualimpl allow clients to use either the remote tagger or the local based on
// their configuration
package dualimpl

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	local "github.com/DataDog/datadog-agent/comp/core/tagger/impl"
	remote "github.com/DataDog/datadog-agent/comp/core/tagger/impl-remote"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
)

// Requires contains the dependencies for the dual tagger component
type Requires struct {
	Lc           compdef.Lifecycle
	RemoteParams tagger.RemoteParams
	DualParams   tagger.DualParams
	Config       config.Component
	Log          log.Component
	Wmeta        workloadmeta.Component
	Telemetry    telemetry.Component
	IPC          ipc.Component
}

// Provides contains returned values for the  dual tagger component
type Provides struct {
	local.Provides
}

// NewComponent returns either a remote tagger or a local tagger based on the configuration
func NewComponent(req Requires) (Provides, error) {
	if req.DualParams.UseRemote(req.Config) {
		remoteRequires := remote.Requires{
			Lc:        req.Lc,
			Params:    req.RemoteParams,
			Config:    req.Config,
			Log:       req.Log,
			Telemetry: req.Telemetry,
			IPC:       req.IPC,
		}

		provide, err := remote.NewComponent(remoteRequires)
		if err != nil {
			return Provides{}, err
		}

		return Provides{
			local.Provides{
				Comp:     provide.Comp,
				Endpoint: provide.Endpoint,
			},
		}, nil
	}

	localRequires := local.Requires{
		Config:       req.Config,
		Telemetry:    req.Telemetry,
		WorkloadMeta: req.Wmeta,
		Lc:           req.Lc,
		Log:          req.Log,
	}
	provide, err := local.NewComponent(localRequires)

	if err != nil {
		return Provides{}, err
	}

	return Provides{
		provide,
	}, nil
}
