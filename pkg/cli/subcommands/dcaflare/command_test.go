// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && kubeapiserver

package dcaflare

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestResolveDCALogFile(t *testing.T) {
	t.Run("unconfigured log_file falls back to the cluster-agent's default log file", func(t *testing.T) {
		cfg := configmock.New(t)
		require.Equal(t, defaultpaths.GetDefaultDCALogFile(), resolveDCALogFile(cfg))
	})

	t.Run("explicitly configured log_file is preserved", func(t *testing.T) {
		cfg := configmock.New(t)
		cfg.Set("log_file", "/custom/cluster-agent.log", pkgconfigmodel.SourceAgentRuntime)
		assert.Equal(t, "/custom/cluster-agent.log", resolveDCALogFile(cfg))
	})
}

func TestCommand(t *testing.T) {
	commands := []*cobra.Command{
		MakeCommand(func() GlobalParams {
			return GlobalParams{}
		}),
	}

	fxutil.TestOneShotSubcommand(t,
		commands,
		[]string{"flare"},
		run,
		func() {})
}

// Verifies the bearer token does not leak to the unauthenticated DCA pprof endpoint (DCA flare).
func TestReadProfileDataNoBearer(t *testing.T) {
	var capturedAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		fmt.Fprint(w, "pprof data")
	}))
	defer ts.Close()

	u, err := url.Parse(ts.URL)
	require.NoError(t, err)
	port, err := strconv.Atoi(u.Port())
	require.NoError(t, err)

	cfg := configmock.New(t)
	cfg.SetInTest("expvar_port", port)

	ipcComp := ipcmock.New(t)
	_, err = readProfileData(ipcComp.GetClient(), 1)
	require.NoError(t, err)
	assert.Empty(t, capturedAuth, "Bearer token must not be sent to unauthenticated DCA pprof endpoint")
}
