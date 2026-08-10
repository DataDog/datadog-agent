// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package apiserverimpl_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipchttp "github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	logcomp "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	secretsmock "github.com/DataDog/datadog-agent/comp/core/secrets/mock"
	settingsmock "github.com/DataDog/datadog-agent/comp/core/settings/mock"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/comp/core/status/statusimpl"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafx "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx"
	apiserver "github.com/DataDog/datadog-agent/comp/process/apiserver/def"
	apiserverfx "github.com/DataDog/datadog-agent/comp/process/apiserver/fx"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/testutil"
)

// startAPIServer starts the apiserver fx app on a freshly-picked port,
// retrying with a new port if the previous pick lost the bind race. Unlike
// probing a port and reusing its number later, each attempt is a real bind,
// so there's no gap between checking and using the port.
func startAPIServer(t *testing.T, buildOpts func(port int) []fx.Option) (apiserver.Component, int) {
	t.Helper()
	var lastErr error
	for attempt := 0; attempt < testutil.MaxBindAttempts; attempt++ {
		port := testutil.FreeTCPPort(t)

		var comp apiserver.Component
		app := fx.New(
			fxutil.FxAgentBase(),
			fx.Supply(fx.Annotate(t, fx.As(new(testing.TB)))),
			fx.Populate(&comp),
			fx.Options(buildOpts(port)...),
		)

		startCtx, cancel := context.WithTimeout(context.Background(), fx.DefaultTimeout)
		err := app.Start(startCtx)
		cancel()
		if err == nil {
			t.Cleanup(func() {
				stopCtx, cancel := context.WithTimeout(context.Background(), fx.DefaultTimeout)
				defer cancel()
				require.NoError(t, app.Stop(stopCtx))
			})
			return comp, port
		}
		if !testutil.IsAddrInUse(err) {
			t.Fatalf("failed to start apiserver: %v", err)
		}
		lastErr = err
	}
	t.Fatalf("could not bind apiserver after %d attempts: %v", testutil.MaxBindAttempts, lastErr)
	return nil, 0
}

func TestLifecycle(t *testing.T) {
	var ipcComp ipc.Component

	_, port := startAPIServer(t, func(port int) []fx.Option {
		return []fx.Option{
			apiserverfx.Module(),
			fx.Provide(func(t testing.TB) logcomp.Component { return logmock.New(t) }),
			fx.Provide(func(t testing.TB) config.Component {
				return config.NewMockWithOverrides(t, map[string]interface{}{
					"process_config.cmd_port": port,
				})
			}),
			workloadmetafx.Module(workloadmeta.NewParams()),
			fx.Supply(
				status.Params{
					PythonVersionGetFunc: func() string { return "n/a" },
				},
			),
			fx.Provide(func() tagger.Component { return taggerfxmock.SetupFakeTagger(t) }),
			statusimpl.Module(),
			settingsmock.MockModule(),
			fx.Provide(func() ipc.Component { return ipcmock.New(t) }),
			fx.Populate(&ipcComp),
			fx.Provide(func() secrets.Component { return secretsmock.New(t) }),
		}
	})

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		url := fmt.Sprintf("https://localhost:%d/agent/status", port)
		_, err := ipcComp.GetClient().Get(url, ipchttp.WithCloseConnection)
		require.NoError(c, err)
	}, 5*time.Second, time.Second)
}

func TestPostAuthentication(t *testing.T) {
	var ipcComp ipc.Component

	_, port := startAPIServer(t, func(port int) []fx.Option {
		return []fx.Option{
			apiserverfx.Module(),
			fx.Provide(func(t testing.TB) logcomp.Component { return logmock.New(t) }),
			fx.Provide(func(t testing.TB) config.Component {
				return config.NewMockWithOverrides(t, map[string]interface{}{
					"process_config.cmd_port": port,
				})
			}),
			workloadmetafx.Module(workloadmeta.NewParams()),
			fx.Supply(
				status.Params{
					PythonVersionGetFunc: func() string { return "n/a" },
				},
			),
			fx.Provide(func() tagger.Component { return taggerfxmock.SetupFakeTagger(t) }),
			statusimpl.Module(),
			settingsmock.MockModule(),
			fx.Provide(func() ipc.Component { return ipcmock.New(t) }),
			fx.Populate(&ipcComp),
			fx.Provide(func() secrets.Component { return secretsmock.New(t) }),
		}
	})

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		// No authentication
		url := fmt.Sprintf("https://localhost:%d/config/log_level?value=debug", port)
		req, err := http.NewRequest("POST", url, nil)
		require.NoError(c, err)
		log.Infof("Issuing unauthenticated test request to url: %s", url)
		_, err = ipcComp.GetClient().Do(req)
		require.NoError(c, err)
		log.Info("Received unauthenticated test response")

		// With authentication
		token := ipcComp.GetAuthToken()
		req.Header.Set("Authorization", "Bearer "+token)
		log.Infof("Issuing authenticated test request to url: %s", url)
		_, err = ipcComp.GetClient().Do(req)
		require.NoError(c, err)
		log.Info("Received authenticated test response")
	}, 5*time.Second, time.Second)
}
