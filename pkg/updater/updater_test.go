// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package updater

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config/remote/client"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/updater/repository"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
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

func (c *testRemoteConfigClient) SetUpdaterPackagesState(packages []*pbgo.PackageState) {
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
	repository := &repository.Repository{
		RootPath:  t.TempDir(),
		LocksPath: t.TempDir(),
	}
	rc := &remoteConfig{client: rcc}
	u := newUpdater(rc, repository, defaultFixture.pkg)
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

	err := updater.bootstrapStable(context.Background())
	assert.NoError(t, err)

	state, err := updater.GetState()
	assert.NoError(t, err)
	assert.Equal(t, fixtureSimpleV1.version, state.Stable)
	assert.False(t, state.HasExperiment())
	assertEqualFS(t, s.PackageFS(fixtureSimpleV1), updater.repository.StableFS())
}

func TestUpdaterBootstrapCatalogUpdate(t *testing.T) {
	s := newTestFixturesServer(t)
	defer s.Close()
	rc := newTestRemoteConfigClient()
	updater := newTestUpdater(t, s, rc, fixtureSimpleV1)
	updater.catalog = catalog{}

	err := updater.bootstrapStable(context.Background())
	assert.Error(t, err)
	rc.SubmitCatalog(s.Catalog())
	err = updater.bootstrapStable(context.Background())
	assert.NoError(t, err)
}

func TestUpdaterStartExperiment(t *testing.T) {
	s := newTestFixturesServer(t)
	defer s.Close()
	rc := newTestRemoteConfigClient()
	updater := newTestUpdater(t, s, rc, fixtureSimpleV1)

	err := updater.bootstrapStable(context.Background())
	assert.NoError(t, err)
	rc.SubmitRequest(remoteAPIRequest{
		ID: uuid.NewString(),
		ExpectedState: expectedState{
			Stable: fixtureSimpleV1.version,
		},
		Method: methodStartExperiment,
		Params: json.RawMessage(`{"version":"` + fixtureSimpleV2.version + `"}`),
	})

	state, err := updater.GetState()
	assert.NoError(t, err)
	assert.Equal(t, fixtureSimpleV1.version, state.Stable)
	assert.Equal(t, fixtureSimpleV2.version, state.Experiment)
	assertEqualFS(t, s.PackageFS(fixtureSimpleV1), updater.repository.StableFS())
	assertEqualFS(t, s.PackageFS(fixtureSimpleV2), updater.repository.ExperimentFS())
}

func TestUpdaterPromoteExperiment(t *testing.T) {
	s := newTestFixturesServer(t)
	defer s.Close()
	rc := newTestRemoteConfigClient()
	updater := newTestUpdater(t, s, rc, fixtureSimpleV1)

	err := updater.bootstrapStable(context.Background())
	assert.NoError(t, err)
	rc.SubmitRequest(remoteAPIRequest{
		ID: uuid.NewString(),
		ExpectedState: expectedState{
			Stable: fixtureSimpleV1.version,
		},
		Method: methodStartExperiment,
		Params: json.RawMessage(`{"version":"` + fixtureSimpleV2.version + `"}`),
	})
	rc.SubmitRequest(remoteAPIRequest{
		ID: uuid.NewString(),
		ExpectedState: expectedState{
			Stable:     fixtureSimpleV1.version,
			Experiment: fixtureSimpleV2.version,
		},
		Method: methodPromoteExperiment,
	})

	state, err := updater.GetState()
	assert.NoError(t, err)
	assert.Equal(t, fixtureSimpleV2.version, state.Stable)
	assert.False(t, state.HasExperiment())
	assertEqualFS(t, s.PackageFS(fixtureSimpleV2), updater.repository.StableFS())
}

func TestUpdaterStopExperiment(t *testing.T) {
	s := newTestFixturesServer(t)
	defer s.Close()
	rc := newTestRemoteConfigClient()
	updater := newTestUpdater(t, s, rc, fixtureSimpleV1)

	err := updater.bootstrapStable(context.Background())
	assert.NoError(t, err)
	rc.SubmitRequest(remoteAPIRequest{
		ID: uuid.NewString(),
		ExpectedState: expectedState{
			Stable: fixtureSimpleV1.version,
		},
		Method: methodStartExperiment,
		Params: json.RawMessage(`{"version":"` + fixtureSimpleV2.version + `"}`),
	})
	state, err := updater.GetState()
	assert.NoError(t, err)
	assert.True(t, state.HasExperiment())
	rc.SubmitRequest(remoteAPIRequest{
		ID: uuid.NewString(),
		ExpectedState: expectedState{
			Stable:     fixtureSimpleV1.version,
			Experiment: fixtureSimpleV2.version,
		},
		Method: methodStopExperiment,
	})

	state, err = updater.GetState()
	assert.NoError(t, err)
	assert.Equal(t, fixtureSimpleV1.version, state.Stable)
	assert.False(t, state.HasExperiment())
	assertEqualFS(t, s.PackageFS(fixtureSimpleV1), updater.repository.StableFS())
}
