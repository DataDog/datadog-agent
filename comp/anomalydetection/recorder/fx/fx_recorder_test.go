// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build anomalydetection_recorder && python

package fx

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	recorder "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

func TestRecorderModuleGraph(t *testing.T) {
	cfg := config.NewMock(t)
	err := fx.ValidateApp(
		Module().Option,
		fx.Provide(func() config.Component { return cfg }),
		fx.Invoke(func(_ option.Option[recorder.Component]) {}),
	)
	require.NoError(t, err)
}
