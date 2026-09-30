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
	recorderimpl "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/impl"
	recorderparquet "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/parquet"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// Module wires the optional recorder with its Parquet writer backend.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(func() option.Option[recorder.WriterFactory] {
			return option.New[recorder.WriterFactory](recorderparquet.Factory{})
		}),
		fxutil.ProvideComponentConstructor(recorderimpl.NewConfiguredComponent),
	)
}
