// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package fx provides the fx module for the noop tagger component
package fx

import (
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	nooptaggerimpl "github.com/DataDog/datadog-agent/comp/core/tagger/impl-noop"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			nooptaggerimpl.NewComponent,
		),
		fxutil.ProvideOptional[tagger.Component](),
	)
}
