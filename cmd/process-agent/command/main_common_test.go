// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package command

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logcomp "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	pidfx "github.com/DataDog/datadog-agent/comp/core/pid/fx"
	pidimpl "github.com/DataDog/datadog-agent/comp/core/pid/impl"
	agent "github.com/DataDog/datadog-agent/comp/process/agent/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type fakeProcessAgent struct {
	enabled bool
}

func (a fakeProcessAgent) Enabled() bool {
	return a.enabled
}

func newRequireEnabledApp(t *testing.T, pidFilePath string, enabled bool) *fx.App {
	return fx.New(
		fx.NopLogger,
		fxutil.FxLifecycleAdapter(),
		fx.Supply(pidimpl.NewParams(pidFilePath)),
		pidfx.Module(),
		fx.Provide(func() logcomp.Component { return logmock.New(t) }),
		fx.Provide(func() config.Component {
			cfg := config.NewMock(t)
			cfg.SetInTest("process_config.process_collection.enabled", false)
			cfg.SetInTest("language_detection.enabled", false)
			return cfg
		}),
		fx.Provide(func() agent.Component { return fakeProcessAgent{enabled: enabled} }),
		requireEnabled(),
	)
}

func TestRequireEnabledDisabledDoesNotWritePIDFile(t *testing.T) {
	pidFilePath := filepath.Join(t.TempDir(), "process-agent.pid")

	app := newRequireEnabledApp(t, pidFilePath, false)

	err := app.Start(context.Background())
	require.ErrorIs(t, err, errAgentDisabled)
	assert.NoFileExists(t, pidFilePath)
}

func TestRequireEnabledEnabledWritesAndRemovesPIDFile(t *testing.T) {
	pidFilePath := filepath.Join(t.TempDir(), "process-agent.pid")

	app := newRequireEnabledApp(t, pidFilePath, true)

	require.NoError(t, app.Start(context.Background()))
	assert.FileExists(t, pidFilePath)

	require.NoError(t, app.Stop(context.Background()))
	assert.NoFileExists(t, pidFilePath)
}
