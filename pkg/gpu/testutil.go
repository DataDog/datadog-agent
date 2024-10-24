// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf && test

package gpu

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func getMockWorkloadMetaStore(t *testing.T) workloadmetamock.Mock {
	return fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		fx.Provide(func() log.Component { return logmock.New(t) }),
		config.MockModule(),
		fx.Supply(context.Background()),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
}

func getSystemContextForTest(t *testing.T) *systemContext {
	sysCtx, err := getSystemContext(testutil.GetBasicNvmlMock(), getMockWorkloadMetaStore(t))
	require.NoError(t, err)
	require.NotNil(t, sysCtx)

	return sysCtx
}

func getMockProbeDependencies(t *testing.T) ProbeDependencies {
	return ProbeDependencies{
		NvmlLib:      testutil.GetBasicNvmlMock(),
		Workloadmeta: getMockWorkloadMetaStore(t),
	}
}
