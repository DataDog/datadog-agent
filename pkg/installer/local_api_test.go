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
	"runtime"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/installer/packages/fixtures"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testLocalAPI struct {
	s *localAPIImpl
	c *localAPIClientImpl
}

func newTestLocalAPI(t *testing.T, s *testDownloadServer) *testLocalAPI {
	rc := newTestRemoteConfigClient()
	installer := newTestInstaller(t, s, rc, fixtures.FixtureSimpleV1)
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
	if runtime.GOOS == "darwin" {
		t.Skip("FIXME: broken on darwin")
	}

	s := newTestDownloadServer(t)
	defer s.Close()
	api := newTestLocalAPI(t, s)
	defer api.Stop()

	// test bootstrap
	err := api.c.BootstrapVersion(fixtures.FixtureSimpleV1.Package, fixtures.FixtureSimpleV1.Version)
	assert.NoError(t, err)

	state, err := api.c.Status()
	assert.NoError(t, err)
	assert.Len(t, state.Packages, 1)
	assert.Contains(t, state.Packages, fixtures.FixtureSimpleV1.Package)
	pkg := state.Packages[fixtures.FixtureSimpleV1.Package]
	assert.Equal(t, fixtures.FixtureSimpleV1.Version, pkg.Stable)
	assert.Empty(t, pkg.Experiment)

	// test start experiment
	err = api.c.StartExperiment(fixtures.FixtureSimpleV2.Package, fixtures.FixtureSimpleV2.Version)
	assert.NoError(t, err)

	state, err = api.c.Status()
	assert.NoError(t, err)
	assert.Len(t, state.Packages, 1)
	assert.Contains(t, state.Packages, fixtures.FixtureSimpleV2.Package)
	pkg = state.Packages[fixtures.FixtureSimpleV2.Package]
	assert.Equal(t, fixtures.FixtureSimpleV1.Version, pkg.Stable)
	assert.Equal(t, fixtures.FixtureSimpleV2.Version, pkg.Experiment)

	// test stop experiment
	err = api.c.StopExperiment(fixtures.FixtureSimpleV2.Package)
	assert.NoError(t, err)

	state, err = api.c.Status()
	assert.NoError(t, err)
	assert.Len(t, state.Packages, 1)
	assert.Contains(t, state.Packages, fixtures.FixtureSimpleV2.Package)
	pkg = state.Packages[fixtures.FixtureSimpleV2.Package]
	assert.Equal(t, fixtures.FixtureSimpleV1.Version, pkg.Stable)
	assert.Empty(t, pkg.Experiment)

	// test promote experiment
	err = api.c.StartExperiment(fixtures.FixtureSimpleV2.Package, fixtures.FixtureSimpleV2.Version)
	assert.NoError(t, err)
	err = api.c.PromoteExperiment(fixtures.FixtureSimpleV2.Package)
	assert.NoError(t, err)

	state, err = api.c.Status()
	assert.NoError(t, err)
	assert.Len(t, state.Packages, 1)
	assert.Contains(t, state.Packages, fixtures.FixtureSimpleV2.Package)
	pkg = state.Packages[fixtures.FixtureSimpleV2.Package]
	assert.Equal(t, fixtures.FixtureSimpleV2.Version, pkg.Stable)
	assert.Empty(t, pkg.Experiment)
}
