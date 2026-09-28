// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build anomalydetection_recorder && python

package fx

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	recorder "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

func TestRecorderModuleGraph(t *testing.T) {
	cfg := config.NewMock(t)
	err := fx.ValidateApp(
		fxutil.FxLifecycleAdapter(),
		Module().Option,
		fx.Provide(func() config.Component { return cfg }),
		fx.Invoke(func(_ option.Option[recorder.Component]) {}),
	)
	require.NoError(t, err)
}

func TestRecorderModuleDisabledProvidesNone(t *testing.T) {
	outputDir := filepath.Join(t.TempDir(), "recordings")
	cfg := config.NewMockWithOverrides(t, map[string]interface{}{
		"anomaly_detection.recording.enabled":    false,
		"anomaly_detection.recording.output_dir": outputDir,
	})
	var provided option.Option[recorder.Component]
	app := fx.New(
		fx.NopLogger,
		fxutil.FxLifecycleAdapter(),
		Module().Option,
		fx.Provide(func() config.Component { return cfg }),
		fx.Populate(&provided),
	)
	require.NoError(t, app.Err())
	_, present := provided.Get()
	require.False(t, present)
	_, err := os.Stat(outputDir)
	require.True(t, errors.Is(err, os.ErrNotExist))
}

func TestRecorderModuleEnabledStopsWriters(t *testing.T) {
	cfg := config.NewMockWithOverrides(t, map[string]interface{}{
		"anomaly_detection.recording.enabled":    true,
		"anomaly_detection.recording.output_dir": t.TempDir(),
	})
	var provided option.Option[recorder.Component]
	app := fx.New(
		fx.NopLogger,
		fxutil.FxLifecycleAdapter(),
		Module().Option,
		fx.Provide(func() config.Component { return cfg }),
		fx.Populate(&provided),
	)
	require.NoError(t, app.Err())
	_, present := provided.Get()
	require.True(t, present)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, app.Start(ctx))
	require.NoError(t, app.Stop(ctx))
}
