// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package apiserver

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/api/authtoken/fetchonlyimpl"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/settings/settingsimpl"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/comp/core/status/statusimpl"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggerfx "github.com/DataDog/datadog-agent/comp/core/tagger/fx"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafx "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestLifecycle(t *testing.T) {
	_ = fxutil.Test[Component](t, fx.Options(
		Module(),
		core.MockBundle(),
		fx.Replace(config.MockParams{Overrides: map[string]interface{}{
			"process_config.cmd_port": 43424,
		}}),
		workloadmetafx.Module(workloadmeta.NewParams()),
		fx.Supply(
			status.Params{
				PythonVersionGetFunc: func() string { return "n/a" },
			},
		),
		taggerfx.Module(tagger.Params{
			UseFakeTagger: true,
		}),
		statusimpl.Module(),
		settingsimpl.MockModule(),
		fetchonlyimpl.MockModule(),
	))

	assert.Eventually(t, func() bool {
		res, err := http.Get("http://localhost:6162/config")
		if err != nil {
			return false
		}
		defer res.Body.Close()

		return res.StatusCode == http.StatusOK
	}, 5*time.Second, time.Second)
}

func TestPostAuthentication(t *testing.T) {
	_ = fxutil.Test[Component](t, fx.Options(
		Module(),
		core.MockBundle(),
		fx.Replace(config.MockParams{Overrides: map[string]interface{}{
			"process_config.cmd_port": 43424,
		}}),
		workloadmetafx.Module(workloadmeta.NewParams()),
		fx.Supply(
			status.Params{
				PythonVersionGetFunc: func() string { return "n/a" },
			},
		),
		taggerfx.Module(tagger.Params{
			UseFakeTagger: true,
		}),
		statusimpl.Module(),
		settingsimpl.MockModule(),
	))

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		// No authentication
		req, err := http.NewRequest("POST", "http://localhost:43424/config/log_level?value=debug", nil)
		require.NoError(c, err)
		res, err := util.GetClient(false).Do(req)
		require.NoError(c, err)
		defer res.Body.Close()
		assert.Equal(c, http.StatusUnauthorized, res.StatusCode)

		// With authentication
		util.CreateAndSetAuthToken(pkgconfigsetup.Datadog())
		req.Header.Set("Authorization", "Bearer "+util.GetAuthToken())
		res, err = util.GetClient(false).Do(req)
		require.NoError(c, err)
		defer res.Body.Close()
		assert.Equal(c, http.StatusOK, res.StatusCode)
	}, 5*time.Second, time.Second)
}
