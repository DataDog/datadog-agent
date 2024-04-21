// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// for now the installer is not supported on windows
//go:build !windows

package installer

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/remote/client"
	"github.com/DataDog/datadog-agent/pkg/installer/packages/fixtures"
	"github.com/DataDog/datadog-agent/pkg/installer/packages/service"
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

func newTestInstaller(t *testing.T, s *testDownloadServer, rcc *testRemoteConfigClient, defaultFixture fixtures.Fixture) *installerImpl {
	u, _, _ := newTestInstallerWithPaths(t, s, rcc, defaultFixture)
	return u
}

func newTestInstallerWithPaths(t *testing.T, s *testDownloadServer, rcc *testRemoteConfigClient, defaultFixture fixtures.Fixture) (*installerImpl, string, string) {
	cfg := model.NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))
	var b = true
	cfg.Set("updater.remote_updates", &b, model.SourceDefault)
	rc := &remoteConfig{client: rcc}
	rootPath := t.TempDir()
	locksPath := t.TempDir()
	u, err := newInstaller(rc, rootPath, locksPath, cfg)
	u.packageManager = &newTestPackageManager(t, rootPath, locksPath).packageManager
	assert.NoError(t, err)
	u.packageManager.configsDir = t.TempDir()
	assert.Nil(t, service.BuildHelperForTests(rootPath, t.TempDir(), true))
	u.catalog = s.Catalog()
	u.bootstrapVersions[defaultFixture.Package] = defaultFixture.Version
	u.Start(context.Background())
	return u, rootPath, locksPath
}

func TestBootstrapDefault(t *testing.T) {
	s := newTestDownloadServer(t)
	defer s.Close()
	rc := newTestRemoteConfigClient()
	installer := newTestInstaller(t, s, rc, fixtures.FixtureSimpleV1)

	err := installer.BootstrapDefault(context.Background(), fixtures.FixtureSimpleV1.Package)
	assert.NoError(t, err)

	r := installer.repositories.Get(fixtures.FixtureSimpleV1.Package)
	state, err := r.GetState()
	assert.NoError(t, err)
	assert.Equal(t, fixtures.FixtureSimpleV1.Version, state.Stable)
	assert.False(t, state.HasExperiment())
	assertEqualFS(t, s.PackageFS(fixtures.FixtureSimpleV1), r.StableFS())
}

func TestBootstrapURL(t *testing.T) {
	s := newTestDownloadServer(t)
	defer s.Close()
	rc := newTestRemoteConfigClient()
	installer := newTestInstaller(t, s, rc, fixtures.FixtureSimpleV1)

	err := installer.BootstrapURL(context.Background(), s.Package(fixtures.FixtureSimpleV1).URL)
	assert.NoError(t, err)

	r := installer.repositories.Get(fixtures.FixtureSimpleV1.Package)
	state, err := r.GetState()
	assert.NoError(t, err)
	assert.Equal(t, fixtures.FixtureSimpleV1.Version, state.Stable)
	assert.False(t, state.HasExperiment())
	assertEqualFS(t, s.PackageFS(fixtures.FixtureSimpleV1), r.StableFS())
}

func TestPurge(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("FIXME: broken on darwin")
	}

	s := newTestDownloadServer(t)
	defer s.Close()
	rc := newTestRemoteConfigClient()
	installer, rootPath, locksPath := newTestInstallerWithPaths(t, s, rc, fixtures.FixtureSimpleV1)

	bootstrapAndAssert := func() {
		err := installer.BootstrapDefault(context.Background(), fixtures.FixtureSimpleV1.Package)
		assert.NoError(t, err)

		r := installer.repositories.Get(fixtures.FixtureSimpleV1.Package)
		state, err := r.GetState()
		assert.NoError(t, err)
		assert.Equal(t, fixtures.FixtureSimpleV1.Version, state.Stable)
		assert.False(t, state.HasExperiment())
		assertEqualFS(t, s.PackageFS(fixtures.FixtureSimpleV1), r.StableFS())
	}
	bootstrapAndAssert()
	assert.Nil(t, os.WriteFile(filepath.Join(locksPath, "not_empty"), []byte("morbier\n"), 0644))
	assertDirNotEmpty(t, locksPath)
	assertDirNotEmpty(t, rootPath)
	purge(testCtx, locksPath, rootPath)
	assertDirExistAndEmpty(t, locksPath)
	assertDirExistAndEmpty(t, rootPath)
	bootstrapAndAssert()
	assertDirNotEmpty(t, rootPath)
}

func assertDirNotEmpty(t *testing.T, path string) {
	_, err := os.Stat(path)
	assert.Nil(t, err)
	entry, err := os.ReadDir(path)
	assert.Nil(t, err)
	assert.NotEmpty(t, entry)
}

func assertDirExistAndEmpty(t *testing.T, path string) {
	_, err := os.Stat(path)
	assert.Nil(t, err)
	entry, err := os.ReadDir(path)
	assert.Nil(t, err)
	assert.Len(t, entry, 0)
}

func TestBootstrapWithRC(t *testing.T) {
	s := newTestDownloadServer(t)
	defer s.Close()
	rc := newTestRemoteConfigClient()
	installer := newTestInstaller(t, s, rc, fixtures.FixtureSimpleV1)

	rc.SubmitRequest(remoteAPIRequest{
		ID:      uuid.NewString(),
		Package: fixtures.FixtureSimpleV2.Package,
		Method:  methodBootstrap,
		Params:  json.RawMessage(`{"version":"` + fixtures.FixtureSimpleV2.Version + `"}`),
	})
	installer.requestsWG.Wait()

	r := installer.repositories.Get(fixtures.FixtureSimpleV2.Package)
	state, err := r.GetState()
	assert.NoError(t, err)
	assert.Equal(t, fixtures.FixtureSimpleV2.Version, state.Stable)
	assert.False(t, state.HasExperiment())
	assertEqualFS(t, s.PackageFS(fixtures.FixtureSimpleV2), r.StableFS())
}

// hacky name to avoid hitting https://github.com/golang/go/issues/62614
func TestBootUpd(t *testing.T) {
	s := newTestDownloadServer(t)
	defer s.Close()
	rc := newTestRemoteConfigClient()
	installer := newTestInstaller(t, s, rc, fixtures.FixtureSimpleV1)
	installer.catalog = catalog{}

	err := installer.BootstrapDefault(context.Background(), fixtures.FixtureSimpleV1.Package)
	assert.Error(t, err)
	rc.SubmitCatalog(s.Catalog())
	err = installer.BootstrapDefault(context.Background(), fixtures.FixtureSimpleV1.Package)
	assert.NoError(t, err)
}

func TestStartExperiment(t *testing.T) {
	s := newTestDownloadServer(t)
	defer s.Close()
	rc := newTestRemoteConfigClient()
	installer := newTestInstaller(t, s, rc, fixtures.FixtureSimpleV1)

	err := installer.BootstrapDefault(context.Background(), fixtures.FixtureSimpleV1.Package)
	assert.NoError(t, err)
	rc.SubmitRequest(remoteAPIRequest{
		ID:      uuid.NewString(),
		Package: fixtures.FixtureSimpleV1.Package,
		ExpectedState: expectedState{
			Stable: fixtures.FixtureSimpleV1.Version,
		},
		Method: methodStartExperiment,
		Params: json.RawMessage(`{"version":"` + fixtures.FixtureSimpleV2.Version + `"}`),
	})
	installer.requestsWG.Wait()

	r := installer.repositories.Get(fixtures.FixtureSimpleV1.Package)
	state, err := r.GetState()
	assert.NoError(t, err)
	assert.Equal(t, fixtures.FixtureSimpleV1.Version, state.Stable)
	assert.Equal(t, fixtures.FixtureSimpleV2.Version, state.Experiment)
	assertEqualFS(t, s.PackageFS(fixtures.FixtureSimpleV1), r.StableFS())
	assertEqualFS(t, s.PackageFS(fixtures.FixtureSimpleV2), r.ExperimentFS())
}

func TestPromoteExperiment(t *testing.T) {
	s := newTestDownloadServer(t)
	defer s.Close()
	rc := newTestRemoteConfigClient()
	installer := newTestInstaller(t, s, rc, fixtures.FixtureSimpleV1)

	err := installer.BootstrapDefault(context.Background(), fixtures.FixtureSimpleV1.Package)
	assert.NoError(t, err)
	rc.SubmitRequest(remoteAPIRequest{
		ID:      uuid.NewString(),
		Package: fixtures.FixtureSimpleV1.Package,
		ExpectedState: expectedState{
			Stable: fixtures.FixtureSimpleV1.Version,
		},
		Method: methodStartExperiment,
		Params: json.RawMessage(`{"version":"` + fixtures.FixtureSimpleV2.Version + `"}`),
	})
	installer.requestsWG.Wait()
	rc.SubmitRequest(remoteAPIRequest{
		ID:      uuid.NewString(),
		Package: fixtures.FixtureSimpleV1.Package,
		ExpectedState: expectedState{
			Stable:     fixtures.FixtureSimpleV1.Version,
			Experiment: fixtures.FixtureSimpleV2.Version,
		},
		Method: methodPromoteExperiment,
	})
	installer.requestsWG.Wait()

	r := installer.repositories.Get(fixtures.FixtureSimpleV1.Package)
	state, err := r.GetState()
	assert.NoError(t, err)
	assert.Equal(t, fixtures.FixtureSimpleV2.Version, state.Stable)
	assert.False(t, state.HasExperiment())
	assertEqualFS(t, s.PackageFS(fixtures.FixtureSimpleV2), r.StableFS())
}

func TestStopExperiment(t *testing.T) {
	s := newTestDownloadServer(t)
	defer s.Close()
	rc := newTestRemoteConfigClient()
	installer := newTestInstaller(t, s, rc, fixtures.FixtureSimpleV1)

	err := installer.BootstrapDefault(context.Background(), fixtures.FixtureSimpleV1.Package)
	assert.NoError(t, err)
	rc.SubmitRequest(remoteAPIRequest{
		ID:      uuid.NewString(),
		Package: fixtures.FixtureSimpleV1.Package,
		ExpectedState: expectedState{
			Stable: fixtures.FixtureSimpleV1.Version,
		},
		Method: methodStartExperiment,
		Params: json.RawMessage(`{"version":"` + fixtures.FixtureSimpleV2.Version + `"}`),
	})
	installer.requestsWG.Wait()
	r := installer.repositories.Get(fixtures.FixtureSimpleV1.Package)
	state, err := r.GetState()
	assert.NoError(t, err)
	assert.True(t, state.HasExperiment())
	rc.SubmitRequest(remoteAPIRequest{
		ID:      uuid.NewString(),
		Package: fixtures.FixtureSimpleV1.Package,
		ExpectedState: expectedState{
			Stable:     fixtures.FixtureSimpleV1.Version,
			Experiment: fixtures.FixtureSimpleV2.Version,
		},
		Method: methodStopExperiment,
	})
	installer.requestsWG.Wait()

	state, err = r.GetState()
	assert.NoError(t, err)
	assert.Equal(t, fixtures.FixtureSimpleV1.Version, state.Stable)
	assert.False(t, state.HasExperiment())
	assertEqualFS(t, s.PackageFS(fixtures.FixtureSimpleV1), r.StableFS())
}
