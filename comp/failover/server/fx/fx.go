// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package fx provides the fx module for the server component
package fx

import (
	server "github.com/DataDog/datadog-agent/comp/failover/server/def"
	serverimpl "github.com/DataDog/datadog-agent/comp/failover/server/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			serverimpl.NewComponent,
		),
		fxutil.ProvideOptional[server.Component](),
	)
}
