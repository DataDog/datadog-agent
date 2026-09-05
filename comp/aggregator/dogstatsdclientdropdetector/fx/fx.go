// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package fx provides the Fx module for the DogStatsD client drop detector.
package fx

import (
	dogstatsdclientdropdetectorimpl "github.com/DataDog/datadog-agent/comp/aggregator/dogstatsdclientdropdetector/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the Fx options for the DogStatsD client drop detector.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(dogstatsdclientdropdetectorimpl.NewComponent),
	)
}
