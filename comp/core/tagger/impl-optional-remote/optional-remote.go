// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package optionalimpl contains the implementation of the optional remote
// tagger. The optionalimpl allow clients to use either the remote tagger or
// the noop tagger based on their configuration. This should only be used in
// the trace-agent in an Azure App Services (AAS) Extension environment where
// we are confident that the container tagging functionality provided by the
// remote tagger is not needed.
package optionalimpl

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	noop "github.com/DataDog/datadog-agent/comp/core/tagger/impl-noop"
	remote "github.com/DataDog/datadog-agent/comp/core/tagger/impl-remote"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	compdef "github.com/DataDog/datadog-agent/comp/def"
)

// Requires contains the dependencies for the dual tagger component
type Requires struct {
	Lc                   compdef.Lifecycle
	OptionalRemoteParams tagger.OptionalRemoteParams
	RemoteParams         tagger.RemoteParams
	Config               config.Component
	Log                  log.Component
	Telemetry            telemetry.Component
	IPC                  ipc.Component
}

// Provides contains returned values for the  dual tagger component
type Provides struct {
	remote.Provides
}

// NewComponent returns either a remote tagger or a noop tagger based on the configuration
func NewComponent(req Requires) (Provides, error) {
	if req.OptionalRemoteParams.Disable(req.Config) {
		return Provides{
			remote.Provides{
				Comp: noop.NewComponent(),
			},
		}, nil
	}

	remoteRequires := remote.Requires{
		Lc:        req.Lc,
		Params:    req.RemoteParams,
		Config:    req.Config,
		Log:       req.Log,
		Telemetry: req.Telemetry,
		IPC:       req.IPC,
	}

	remoteProvides, err := remote.NewComponent(remoteRequires)
	if err != nil {
		return Provides{}, err
	}

	return Provides{remoteProvides}, nil
}
