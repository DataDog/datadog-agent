// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package traceconfigimpl

import (
	"go.uber.org/fx"
	"testing"

	traceconfig "github.com/DataDog/datadog-agent/comp/trace/config/def"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// MockModule defines the fx options for the mock component.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(NewMock),
		fx.Supply(traceconfig.Params{
			FailIfAPIKeyMissing: true,
		}),
	)
}

// NewMock exported mock builder to allow modifying mocks that might be
// supplied in tests and used for dep injection.
func NewMock(reqs Requires, _ testing.TB) (traceconfig.Component, error) {
	reqs.Config.SetWithoutSource("api_key", "apikey")
	traceCfg, err := setupConfigCommon(reqs)
	if err != nil {
		return nil, err
	}

	c := cfg{
		warnings:    &model.Warnings{},
		coreConfig:  reqs.Config,
		AgentConfig: traceCfg,
		ipc:         reqs.IPC,
	}

	c.SetMaxMemCPU(env.IsContainerized())

	return &c, nil
}
