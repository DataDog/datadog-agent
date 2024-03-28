// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package metadataschedulerimpl provides the metadata scheduler component implementation.
package metadataschedulerimpl

import (
	"context"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/agent/metadatascheduler"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	pkgMetadata "github.com/DataDog/datadog-agent/pkg/metadata"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newMetadataScheduler),
	)
}

type dependencies struct {
	fx.In
	Demultiplexer demultiplexer.Component
	Lc            fx.Lifecycle
}

func newMetadataScheduler(deps dependencies) (metadatascheduler.Component, error) {
	metadataScheduler := pkgMetadata.NewScheduler(deps.Demultiplexer)
	if err := pkgMetadata.SetupMetadataCollection(metadataScheduler, pkgMetadata.AllDefaultCollectors); err != nil {
		return struct{}{}, err
	}
	deps.Lc.Append(fx.Hook{
		OnStop: func(context.Context) error {
			metadataScheduler.Stop()
			return nil
		}})
	return struct{}{}, nil
}
