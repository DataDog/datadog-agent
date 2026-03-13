// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package fx provides the fx module for the connectionsforwarder component
package fx

import (
	connectionsforwarder "github.com/DataDog/datadog-agent/comp/forwarder/connectionsforwarder/def"
	connectionsforwarderimpl "github.com/DataDog/datadog-agent/comp/forwarder/connectionsforwarder/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			connectionsforwarderimpl.NewComponent,
		),
		fxutil.ProvideOptional[connectionsforwarder.Component](),
	)
}
