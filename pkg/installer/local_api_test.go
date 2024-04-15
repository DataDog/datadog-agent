// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// for now the installer is not supported on windows
//go:build !windows

package installer

import (
	"context"
	"net"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testLocalAPI struct {
	s *localAPIImpl
	c *localAPIClientImpl
}

func newTestLocalAPI(t *testing.T, s *testFixturesServer) *testLocalAPI {
	rc := newTestRemoteConfigClient()
	installer := newTestInstaller(t, s, rc, fixtureSimpleV1)
	rc.SubmitCatalog(s.Catalog())
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	apiServer := &localAPIImpl{
		server:    &http.Server{},
		listener:  l,
		installer: installer,
	}
	apiServer.Start(context.Background())
	apiClient := &localAPIClientImpl{
		client: &http.Client{},
		addr:   l.Addr().String(),
	}
	return &testLocalAPI{apiServer, apiClient}
}

func (api *testLocalAPI) Stop() {
	api.s.Stop(context.Background())
}

func TestLocalAPI(t *testing.T) {
	s := newTestFixturesServer(t)
	defer s.Close()
	api := newTestLocalAPI(t, s)
	defer api.Stop()

	// test bootstrap
	err := api.c.BootstrapVersion(fixtureSimpleV1.pkg, fixtureSimpleV1.version)
	assert.NoError(t, err)

	state, err := api.c.Status()
	assert.NoError(t, err)
	assert.Len(t, state.Packages, 1)
	assert.Contains(t, state.Packages, fixtureSimpleV1.pkg)
	pkg := state.Packages[fixtureSimpleV1.pkg]
	assert.Equal(t, fixtureSimpleV1.version, pkg.Stable)
	assert.Empty(t, pkg.Experiment)

	// test start experiment
	err = api.c.StartExperiment(fixtureSimpleV2.pkg, fixtureSimpleV2.version)
	assert.NoError(t, err)

	state, err = api.c.Status()
	assert.NoError(t, err)
	assert.Len(t, state.Packages, 1)
	assert.Contains(t, state.Packages, fixtureSimpleV2.pkg)
	pkg = state.Packages[fixtureSimpleV2.pkg]
	assert.Equal(t, fixtureSimpleV1.version, pkg.Stable)
	assert.Equal(t, fixtureSimpleV2.version, pkg.Experiment)

	// test stop experiment
	err = api.c.StopExperiment(fixtureSimpleV2.pkg)
	assert.NoError(t, err)

	state, err = api.c.Status()
	assert.NoError(t, err)
	assert.Len(t, state.Packages, 1)
	assert.Contains(t, state.Packages, fixtureSimpleV2.pkg)
	pkg = state.Packages[fixtureSimpleV2.pkg]
	assert.Equal(t, fixtureSimpleV1.version, pkg.Stable)
	assert.Empty(t, pkg.Experiment)

	// test promote experiment
	err = api.c.StartExperiment(fixtureSimpleV2.pkg, fixtureSimpleV2.version)
	assert.NoError(t, err)
	err = api.c.PromoteExperiment(fixtureSimpleV2.pkg)
	assert.NoError(t, err)

	state, err = api.c.Status()
	assert.NoError(t, err)
	assert.Len(t, state.Packages, 1)
	assert.Contains(t, state.Packages, fixtureSimpleV2.pkg)
	pkg = state.Packages[fixtureSimpleV2.pkg]
	assert.Equal(t, fixtureSimpleV2.version, pkg.Stable)
	assert.Empty(t, pkg.Experiment)
}
