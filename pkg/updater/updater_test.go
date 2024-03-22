// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// for now the updater is not supported on windows
//go:build !windows

package updater

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config/remote/client"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

type testRemoteConfigClient struct {
	listeners map[string][]client.Handler
}

func newTestRemoteConfigClient() *testRemoteConfigClient {
	return &testRemoteConfigClient{
		listeners: make(map[string][]client.Handler),
	}
}

func (c *testRemoteConfigClient) Start() {
}

func (c *testRemoteConfigClient) Close() {
}

func (c *testRemoteConfigClient) Subscribe(product string, fn func(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus))) {
	c.listeners[product] = append(c.listeners[product], client.Handler(fn))
}

func (c *testRemoteConfigClient) SetUpdaterPackagesState(_ []*pbgo.PackageState) {
}

func (c *testRemoteConfigClient) SubmitCatalog(catalog catalog) {
	rawCatalog, err := json.Marshal(catalog)
	if err != nil {
		panic(err)
	}
	for _, l := range c.listeners[state.ProductUpdaterCatalogDD] {
		l(map[string]state.RawConfig{
			"catalog": {
				Config: rawCatalog,
			},
		}, func(string, state.ApplyStatus) {})
	}
}

func (c *testRemoteConfigClient) SubmitRequest(request remoteAPIRequest) {
	rawTask, err := json.Marshal(request)
	if err != nil {
		panic(err)
	}
	for _, l := range c.listeners[state.ProductUpdaterTask] {
		l(map[string]state.RawConfig{
			"request": {
				Config: rawTask,
			},
		}, func(string, state.ApplyStatus) {})
	}
}

func newTestUpdater(t *testing.T, s *testFixturesServer, rcc *testRemoteConfigClient, defaultFixture fixture) *updaterImpl {
	rc := &remoteConfig{client: rcc}
	u := newUpdater(rc, t.TempDir(), t.TempDir(), "")
	u.installer.configsDir = t.TempDir()
	u.catalog = s.Catalog()
	u.bootstrapVersions[defaultFixture.pkg] = defaultFixture.version
	u.Start(context.Background())
	return u
}

func TestUpdaterBootstrap(t *testing.T) {
	s := newTestFixturesServer(t)
	defer s.Close()
	rc := newTestRemoteConfigClient()
	updater := newTestUpdater(t, s, rc, fixtureSimpleV1)

	err := updater.Bootstrap(context.Background(), fixtureSimpleV1.pkg)
	assert.NoError(t, err)

	r := updater.repositories.Get(fixtureSimpleV1.pkg)
	state, err := r.GetState()
	assert.NoError(t, err)
	assert.Equal(t, fixtureSimpleV1.version, state.Stable)
	assert.False(t, state.HasExperiment())
	assertEqualFS(t, s.PackageFS(fixtureSimpleV1), r.StableFS())
}

func TestUpdaterBootstrapWithRC(t *testing.T) {
	s := newTestFixturesServer(t)
	defer s.Close()
	rc := newTestRemoteConfigClient()
	updater := newTestUpdater(t, s, rc, fixtureSimpleV1)

	rc.SubmitRequest(remoteAPIRequest{
		ID:      uuid.NewString(),
		Package: fixtureSimpleV2.pkg,
		Method:  methodBootstrap,
		Params:  json.RawMessage(`{"version":"` + fixtureSimpleV2.version + `"}`),
	})
	updater.requestsWG.Wait()

	r := updater.repositories.Get(fixtureSimpleV2.pkg)
	state, err := r.GetState()
	assert.NoError(t, err)
	assert.Equal(t, fixtureSimpleV2.version, state.Stable)
	assert.False(t, state.HasExperiment())
	assertEqualFS(t, s.PackageFS(fixtureSimpleV2), r.StableFS())
}

func TestUpdaterBootstrapCatalogUpdate(t *testing.T) {
	s := newTestFixturesServer(t)
	defer s.Close()
	rc := newTestRemoteConfigClient()
	updater := newTestUpdater(t, s, rc, fixtureSimpleV1)
	updater.catalog = catalog{}

	err := updater.Bootstrap(context.Background(), fixtureSimpleV1.pkg)
	assert.Error(t, err)
	rc.SubmitCatalog(s.Catalog())
	err = updater.Bootstrap(context.Background(), fixtureSimpleV1.pkg)
	assert.NoError(t, err)
}

func TestUpdaterStartExperiment(t *testing.T) {
	s := newTestFixturesServer(t)
	defer s.Close()
	rc := newTestRemoteConfigClient()
	updater := newTestUpdater(t, s, rc, fixtureSimpleV1)

	err := updater.Bootstrap(context.Background(), fixtureSimpleV1.pkg)
	assert.NoError(t, err)
	rc.SubmitRequest(remoteAPIRequest{
		ID:      uuid.NewString(),
		Package: fixtureSimpleV1.pkg,
		ExpectedState: expectedState{
			Stable: fixtureSimpleV1.version,
		},
		Method: methodStartExperiment,
		Params: json.RawMessage(`{"version":"` + fixtureSimpleV2.version + `"}`),
	})
	updater.requestsWG.Wait()

	r := updater.repositories.Get(fixtureSimpleV1.pkg)
	state, err := r.GetState()
	assert.NoError(t, err)
	assert.Equal(t, fixtureSimpleV1.version, state.Stable)
	assert.Equal(t, fixtureSimpleV2.version, state.Experiment)
	assertEqualFS(t, s.PackageFS(fixtureSimpleV1), r.StableFS())
	assertEqualFS(t, s.PackageFS(fixtureSimpleV2), r.ExperimentFS())
}

func TestUpdaterPromoteExperiment(t *testing.T) {
	s := newTestFixturesServer(t)
	defer s.Close()
	rc := newTestRemoteConfigClient()
	updater := newTestUpdater(t, s, rc, fixtureSimpleV1)

	err := updater.Bootstrap(context.Background(), fixtureSimpleV1.pkg)
	assert.NoError(t, err)
	rc.SubmitRequest(remoteAPIRequest{
		ID:      uuid.NewString(),
		Package: fixtureSimpleV1.pkg,
		ExpectedState: expectedState{
			Stable: fixtureSimpleV1.version,
		},
		Method: methodStartExperiment,
		Params: json.RawMessage(`{"version":"` + fixtureSimpleV2.version + `"}`),
	})
	updater.requestsWG.Wait()
	rc.SubmitRequest(remoteAPIRequest{
		ID:      uuid.NewString(),
		Package: fixtureSimpleV1.pkg,
		ExpectedState: expectedState{
			Stable:     fixtureSimpleV1.version,
			Experiment: fixtureSimpleV2.version,
		},
		Method: methodPromoteExperiment,
	})
	updater.requestsWG.Wait()

	r := updater.repositories.Get(fixtureSimpleV1.pkg)
	state, err := r.GetState()
	assert.NoError(t, err)
	assert.Equal(t, fixtureSimpleV2.version, state.Stable)
	assert.False(t, state.HasExperiment())
	assertEqualFS(t, s.PackageFS(fixtureSimpleV2), r.StableFS())
}

func TestUpdaterStopExperiment(t *testing.T) {
	s := newTestFixturesServer(t)
	defer s.Close()
	rc := newTestRemoteConfigClient()
	updater := newTestUpdater(t, s, rc, fixtureSimpleV1)

	err := updater.Bootstrap(context.Background(), fixtureSimpleV1.pkg)
	assert.NoError(t, err)
	rc.SubmitRequest(remoteAPIRequest{
		ID:      uuid.NewString(),
		Package: fixtureSimpleV1.pkg,
		ExpectedState: expectedState{
			Stable: fixtureSimpleV1.version,
		},
		Method: methodStartExperiment,
		Params: json.RawMessage(`{"version":"` + fixtureSimpleV2.version + `"}`),
	})
	updater.requestsWG.Wait()
	r := updater.repositories.Get(fixtureSimpleV1.pkg)
	state, err := r.GetState()
	assert.NoError(t, err)
	assert.True(t, state.HasExperiment())
	rc.SubmitRequest(remoteAPIRequest{
		ID:      uuid.NewString(),
		Package: fixtureSimpleV1.pkg,
		ExpectedState: expectedState{
			Stable:     fixtureSimpleV1.version,
			Experiment: fixtureSimpleV2.version,
		},
		Method: methodStopExperiment,
	})
	updater.requestsWG.Wait()

	state, err = r.GetState()
	assert.NoError(t, err)
	assert.Equal(t, fixtureSimpleV1.version, state.Stable)
	assert.False(t, state.HasExperiment())
	assertEqualFS(t, s.PackageFS(fixtureSimpleV1), r.StableFS())
}
