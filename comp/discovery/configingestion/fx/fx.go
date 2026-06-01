// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package fx provides the fx module for the config ingestion component.
package fx

import (
	configingestiondef "github.com/DataDog/datadog-agent/comp/discovery/configingestion/def"
	configingestionimpl "github.com/DataDog/datadog-agent/comp/discovery/configingestion/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the Fx options for the config ingestion component.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			configingestionimpl.NewComponent,
		),
		fxutil.ProvideOptional[configingestiondef.Component](),
	)
}
