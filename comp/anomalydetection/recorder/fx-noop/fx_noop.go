// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package fx provides the fx module for the recorder component.
// This no-op variant provides an empty option so the observer starts
// without recording. Wire the full recorder implementation when recording is needed.
package fx

import (
	"go.uber.org/fx"

	recorder "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// Module defines the fx options for the no-op recorder component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(func() option.Option[recorder.Component] {
			return option.None[recorder.Component]()
		}),
	)
}
