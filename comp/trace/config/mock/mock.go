// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the trace config component.
package mock

import (
	"testing"

	"github.com/stretchr/testify/require"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	taggerimpl "github.com/DataDog/datadog-agent/comp/core/tagger/impl"
	telemetrymock "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetaimpl "github.com/DataDog/datadog-agent/comp/core/workloadmeta/impl"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	traceconfig "github.com/DataDog/datadog-agent/comp/trace/config/def"
	traceconfigimpl "github.com/DataDog/datadog-agent/comp/trace/config/impl"
)

// New returns a mock trace config component.
func New(t testing.TB) traceconfig.Component {
	t.Helper()

	cfg := coreconfig.NewMock(t)
	ipcComp := ipcmock.New(t)
	logComp := logmock.New(t)
	workloadmetaComp := workloadmetaimpl.NewWorkloadMetaMock(workloadmetaimpl.Dependencies{
		Lc:     &compdef.TestLifecycle{},
		Config: cfg,
		Log:    logComp,
		Params: workloadmeta.NewParams(),
	})
	taggerComp := taggerimpl.NewMock(taggerimpl.MockRequires{
		Config:       cfg,
		WorkloadMeta: workloadmetaComp,
		Log:          logComp,
		Telemetry:    telemetrymock.New(t),
	}).Comp
	component, err := traceconfigimpl.NewMock(traceconfigimpl.Requires{
		Params: traceconfig.Params{FailIfAPIKeyMissing: true},
		Config: cfg,
		Tagger: taggerComp,
		IPC:    ipcComp,
	})
	require.NoError(t, err)
	return component
}
