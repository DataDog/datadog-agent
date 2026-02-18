// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package workloadfilterimpl contains the implementation of the filter component.
package workloadfilterimpl

import (
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	logcomp "github.com/DataDog/datadog-agent/comp/core/log/def"
	coretelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry"
	implbase "github.com/DataDog/datadog-agent/comp/core/workloadfilter/baseimpl"
	"github.com/DataDog/datadog-agent/comp/core/workloadfilter/catalog"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
)

// localFilterStore is the local implementation of the workloadfilter component.
type localFilterStore struct {
	*implbase.BaseFilterStore
}

// Requires defines the dependencies of the local workloadfilter component.
type Requires struct {
	compdef.In

	Config    config.Component
	Log       logcomp.Component
	Telemetry coretelemetry.Component
}

// Provides defines the fields provided by the local workloadfilter constructor.
type Provides struct {
	compdef.Out

	Comp          workloadfilter.Component
	FlareProvider flaretypes.Provider
}

// NewComponent returns a new local workloadfilter client
func NewComponent(req Requires) (Provides, error) {
	localFilter := newFilter(req.Config, req.Log, req.Telemetry)

	return Provides{
		Comp:          localFilter,
		FlareProvider: flaretypes.NewProvider(localFilter.FlareCallback),
	}, nil
}

var _ workloadfilter.Component = (*localFilterStore)(nil)

func newFilter(cfg config.Component, logger logcomp.Component, telemetry coretelemetry.Component) *localFilterStore {
	baseFilter := implbase.NewBaseFilterStore(cfg, logger, telemetry)

	localFilter := &localFilterStore{
		BaseFilterStore: baseFilter,
	}

	// Register filter programs that can only be computed locally on the the core Agent.

	// Container Filters
	localFilter.RegisterFactory(workloadfilter.ContainerCELMetrics, catalog.ContainerCELMetricsProgram)
	localFilter.RegisterFactory(workloadfilter.ContainerCELLogs, catalog.ContainerCELLogsProgram)
	localFilter.RegisterFactory(workloadfilter.ContainerCELSBOM, catalog.ContainerCELSBOMProgram)
	localFilter.RegisterFactory(workloadfilter.ContainerCELGlobal, catalog.ContainerCELGlobalProgram)

	// Service Filters
	localFilter.RegisterFactory(workloadfilter.KubeServiceCELMetrics, catalog.ServiceCELMetricsProgram)
	localFilter.RegisterFactory(workloadfilter.KubeServiceCELGlobal, catalog.ServiceCELGlobalProgram)

	// Endpoints Filters
	localFilter.RegisterFactory(workloadfilter.KubeEndpointCELMetrics, catalog.EndpointCELMetricsProgram)
	localFilter.RegisterFactory(workloadfilter.KubeEndpointCELGlobal, catalog.EndpointCELGlobalProgram)

	// Pod Filters
	localFilter.RegisterFactory(workloadfilter.PodCELMetrics, catalog.PodCELMetricsProgram)
	localFilter.RegisterFactory(workloadfilter.PodCELGlobal, catalog.PodCELGlobalProgram)

	// Process Filters
	localFilter.RegisterFactory(workloadfilter.ProcessCELLogs, catalog.ProcessCELLogsProgram)
	localFilter.RegisterFactory(workloadfilter.ProcessCELGlobal, catalog.ProcessCELGlobalProgram)

	return localFilter
}

// Evaluate serves the request to evaluate a program for a given entity.
func (f *localFilterStore) Evaluate(programName string, entity workloadfilter.Filterable) (workloadfilter.Result, error) {
	program := f.GetProgram(entity.Type(), programName)
	if program == nil {
		return workloadfilter.Unknown, fmt.Errorf("program %s not found", programName)
	}
	return program.Evaluate(entity), nil
}
