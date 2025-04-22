// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package apiserver

import (
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/api/authtoken"
	authtokenmock "github.com/DataDog/datadog-agent/comp/api/authtoken/mock"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/secrets/secretsimpl"
	"github.com/DataDog/datadog-agent/comp/core/settings/settingsimpl"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/comp/core/status/statusimpl"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafx "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func TestLifecycle(t *testing.T) {
	listener, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	var at authtoken.Component

	_ = fxutil.Test[Component](t, fx.Options(
		Module(),
		core.MockBundle(),
		fx.Replace(config.MockParams{Overrides: map[string]interface{}{
			"process_config.cmd_port": port,
		}}),
		workloadmetafx.Module(workloadmeta.NewParams()),
		fx.Supply(
			status.Params{
				PythonVersionGetFunc: func() string { return "n/a" },
			},
		),
		fx.Provide(func() tagger.Component { return taggerfxmock.SetupFakeTagger(t) }),
		statusimpl.Module(),
		settingsimpl.MockModule(),
		fx.Provide(func(t testing.TB) authtoken.Component { return authtokenmock.New(t) }),
		fx.Populate(&at),
		secretsimpl.MockModule(),
	))

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		url := fmt.Sprintf("https://localhost:%d/agent/status", port)
		client := util.GetClient()
		_, err := util.DoGet(client, url, util.CloseConnection)
		require.NoError(c, err)
	}, 5*time.Second, time.Second)
}

func TestPostAuthentication(t *testing.T) {
	listener, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	var at authtoken.Component

	_ = fxutil.Test[Component](t, fx.Options(
		Module(),
		core.MockBundle(),
		fx.Replace(config.MockParams{Overrides: map[string]interface{}{
			"process_config.cmd_port": port,
		}}),
		workloadmetafx.Module(workloadmeta.NewParams()),
		fx.Supply(
			status.Params{
				PythonVersionGetFunc: func() string { return "n/a" },
			},
		),
		fx.Provide(func() tagger.Component { return taggerfxmock.SetupFakeTagger(t) }),
		statusimpl.Module(),
		settingsimpl.MockModule(),
		fx.Provide(func(t testing.TB) authtoken.Component { return authtokenmock.New(t) }),
		fx.Populate(&at),
		secretsimpl.MockModule(),
	))

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		// No authentication
		url := fmt.Sprintf("https://localhost:%d/config/log_level?value=debug", port)
		req, err := http.NewRequest("POST", url, nil)
		require.NoError(c, err)
		log.Infof("Issuing unauthenticated test request to url: %s", url)
		res, err := util.GetClient().Do(req)
		require.NoError(c, err)
		defer res.Body.Close()
		log.Info("Received unauthenticated test response")
		assert.Equal(c, http.StatusUnauthorized, res.StatusCode)

		// With authentication
		token, err := at.Get()
		require.NoError(c, err)
		req.Header.Set("Authorization", "Bearer "+token)
		log.Infof("Issuing authenticated test request to url: %s", url)
		res, err = util.GetClient().Do(req)
		require.NoError(c, err)
		defer res.Body.Close()
		log.Info("Received authenticated test response")
		assert.Equal(c, http.StatusOK, res.StatusCode)
	}, 5*time.Second, time.Second)
}
