// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fx provides the fx module for the recorder component.
// Wire this module when full parquet recording is needed (e.g. in the testbench).
// For the main agent build, use recorder/fx-noop instead.
package fx

import (
	recorder "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
	recorderimpl "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			recorderimpl.NewComponent,
		),
		fxutil.ProvideOptional[recorder.Component](),
	)
}
