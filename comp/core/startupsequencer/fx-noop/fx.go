// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fx exposes the no-op startupsequencer to fx.
package fx

import (
	startupsequencer "github.com/DataDog/datadog-agent/comp/core/startupsequencer/def"
	startupsequencernoopimpl "github.com/DataDog/datadog-agent/comp/core/startupsequencer/noop-impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for the no-op startupsequencer.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			startupsequencernoopimpl.NewComponent,
		),
		fxutil.ProvideOptional[startupsequencer.Component](),
	)
}
