// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package apmetwtracerimpl

import (
	"fmt"
	"github.com/DataDog/datadog-agent/comp/apm/etwtracer"
	"github.com/DataDog/datadog-agent/comp/etw"
	"go.uber.org/fx"
	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
var Module = fxutil.Component(
	fx.Provide(newApmEtwTracerImpl),
)

type apmetwtracerimpl struct {
	session                   etw.Session
	dotNetRuntimeProviderGuid windows.GUID

	// PIDs contains the list of PIDs we are interested in
	// We use a map it's more appropriate for frequent add / remove and because struct{} occupies 0 bytes
	pids map[uint64]struct{}
}

func (a *apmetwtracerimpl) AddPID(pid uint64) {
	a.pids[pid] = struct{}{}
	a.reconfigureProvider()
}

func (a *apmetwtracerimpl) RemovePID(pid uint64) {
	delete(a.pids, pid)
	a.reconfigureProvider()
}

func (a *apmetwtracerimpl) reconfigureProvider() error {
	err := a.session.ConfigureProvider(a.dotNetRuntimeProviderGuid, func(cfg *etw.ProviderConfiguration) {
		cfg.TraceLevel = etw.TRACE_LEVEL_VERBOSE
		cfg.MatchAnyKeyword = 0x40004001
		pidsList := make([]uint64, 0, len(a.pids))
		for p := range a.pids {
			pidsList = append(pidsList, p)
		}
		cfg.PIDs = pidsList
	})
	if err != nil {
		return fmt.Errorf("failed to configure the Microsoft-Windows-DotNETRuntime provider: %v", err)
	}
	return nil
}

type dependencies struct {
	fx.In
	Lc  fx.Lifecycle
	etw etw.Component
}

func newApmEtwTracerImpl(deps dependencies) (apmetwtracer.Component, error) {
	etwSessionName := "Datadog APM ETW tracer"
	session, err := deps.etw.NewSession(etwSessionName)
	if err != nil {
		return nil, fmt.Errorf("failed to create the ETW session '%s': %v", etwSessionName, err)
	}

	// Microsoft-Windows-DotNETRuntime = {E13C0D23-CCBC-4E12-931B-D9CC2EEE27E4}
	guid, _ := windows.GUIDFromString("{E13C0D23-CCBC-4E12-931B-D9CC2EEE27E4}")

	apmEtwTracer := &apmetwtracerimpl{
		session:                   session,
		dotNetRuntimeProviderGuid: guid,
	}

	err = apmetwtracerimpl.reconfigureProvider()
	if err != nil {
		return nil, err
	}

	return apmEtwTracer, nil
}
