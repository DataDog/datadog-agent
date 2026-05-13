// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// The `python` build tag is used here as a proxy for "full agent, not IoT agent".
// See comp/anomalydetection/recorder/fx/fx_noop.go for the stub used in IoT agent
// and other size-sensitive builds, which avoids linking apache/arrow-go.

//go:build python

// Package fx provides the fx module for the recorder component.
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
