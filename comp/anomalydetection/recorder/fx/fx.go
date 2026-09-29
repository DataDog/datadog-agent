// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build anomalydetection_recorder && python

// Package fx wires recorder components into tagged Agent builds.
package fx

import (
	"go.uber.org/fx"

	recorder "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// Module supplies the unregistered writer-provider state for this PR.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(func() option.Option[recorder.WriterFactory] {
			return option.None[recorder.WriterFactory]()
		}),
		fx.Provide(func(_ option.Option[recorder.WriterFactory]) option.Option[recorder.Component] {
			return option.None[recorder.Component]()
		}),
	)
}
