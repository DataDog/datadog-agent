// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package fx provides the fx module for the tagger component
package fx

import (
	filter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	filterimpl "github.com/DataDog/datadog-agent/comp/core/workloadfilter/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			filterimpl.NewComponent,
		),
		fxutil.ProvideOptional[filter.Component](),
	)
}
