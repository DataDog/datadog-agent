// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package fx provides the fx module for the optional remote tagger component
// for use in the trace-agent running in an Azure App Services (AAS) Extension.
package fx

import (
	"go.uber.org/fx"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	optionalimpl "github.com/DataDog/datadog-agent/comp/core/tagger/impl-optional-remote"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component. This is intended to be
// used with the trace-agent and only in Azure App Services (AAS) Extension
// environments where we are confident that we do not need the container
// tagging that the remote tagger provides.
func Module(optionalParams tagger.OptionalRemoteParams, remoteParams tagger.RemoteParams) fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			optionalimpl.NewComponent,
		),
		fx.Supply(remoteParams),
		fx.Supply(optionalParams),
		fxutil.ProvideOptional[tagger.Component](),
	)
}
