// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// for now the installer is not supported on windows
//go:build !windows

package daemon

import (
	"context"
	"encoding/json"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/fleet/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/repository"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/version"
)

type testPackageManager struct {
	mock.Mock
}

func (m *testPackageManager) IsInstalled(ctx context.Context, pkg string) (bool, error) {
	args := m.Called(ctx, pkg)
	return args.Bool(0), args.Error(1)
}

func (m *testPackageManager) State(pkg string) (repository.State, error) {
	args := m.Called(pkg)
	return args.Get(0).(repository.State), args.Error(1)
}

func (m *testPackageManager) States() (map[string]repository.State, error) {
	args := m.Called()
	return args.Get(0).(map[string]repository.State), args.Error(1)
}

func (m *testPackageManager) ConfigState(pkg string) (repository.State, error) {
	args := m.Called(pkg)
	return args.Get(0).(repository.State), args.Error(1)
}

func (m *testPackageManager) ConfigStates() (map[string]repository.State, error) {
	args := m.Called()
	return args.Get(0).(map[string]repository.State), args.Error(1)
}

func (m *testPackageManager) Install(ctx context.Context, url string, installArgs []string) error {
	args := m.Called(ctx, url, installArgs)
	return args.Error(0)
}

func (m *testPackageManager) Remove(ctx context.Context, pkg string) error {
	args := m.Called(ctx, pkg)
	return args.Error(0)
}

func (m *testPackageManager) Purge(_ context.Context) {
	panic("not implemented")
}

func (m *testPackageManager) InstallExperiment(ctx context.Context, url string) error {
	args := m.Called(ctx, url)
	return args.Error(0)
}

func (m *testPackageManager) RemoveExperiment(ctx context.Context, pkg string) error {
	args := m.Called(ctx, pkg)
	return args.Error(0)
}

func (m *testPackageManager) PromoteExperiment(ctx context.Context, pkg string) error {
	args := m.Called(ctx, pkg)
	return args.Error(0)
}

func (m *testPackageManager) GarbageCollect(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *testPackageManager) InstrumentAPMInjector(ctx context.Context, method string) error {
	args := m.Called(ctx, method)
	return args.Error(0)
}

func (m *testPackageManager) UninstrumentAPMInjector(ctx context.Context, method string) error {
	args := m.Called(ctx, method)
	return args.Error(0)
}

type testRemoteConfigClient struct {
	sync.Mutex
	t         *testing.T
	listeners map[string][]func(map[string]state.RawConfig, func(cfgPath string, status state.ApplyStatus))
}

func newTestRemoteConfigClient(t *testing.T) *testRemoteConfigClient {
	return &testRemoteConfigClient{
		t:         t,
		listeners: make(map[string][]func(map[string]state.RawConfig, func(cfgPath string, status state.ApplyStatus))),
	}
}

func (c *testRemoteConfigClient) Start() {
}

func (c *testRemoteConfigClient) Close() {
}

func (c *testRemoteConfigClient) Subscribe(product string, fn func(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus))) {
	c.Lock()
	defer c.Unlock()
	c.listeners[product] = append(c.listeners[product], fn)
}

func (c *testRemoteConfigClient) SetInstallerState(_ []*pbgo.PackageState) {
}

func (c *testRemoteConfigClient) GetInstallerState() []*pbgo.PackageState {
	return nil
}

func (c *testRemoteConfigClient) SubmitCatalog(catalog catalog) {
	c.Lock()
	defer c.Unlock()
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

func (c *testRemoteConfigClient) subscribedToRequests() bool {
	c.Lock()
	defer c.Unlock()
	_, ok := c.listeners[state.ProductUpdaterTask]
	return ok
}

func (c *testRemoteConfigClient) SubmitRequest(request remoteAPIRequest) {
	// wait for the client to subscribe to the requests after the catalog has been applied
	require.Eventually(c.t, c.subscribedToRequests, 1*time.Second, 10*time.Millisecond)

	c.Lock()
	defer c.Unlock()
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

type testInstaller struct {
	*daemonImpl
	rcc *testRemoteConfigClient
	pm  *testPackageManager
}

func newTestInstaller(t *testing.T) *testInstaller {
	pm := &testPackageManager{}
	pm.On("States").Return(map[string]repository.State{}, nil)
	pm.On("ConfigStates").Return(map[string]repository.State{}, nil)
	rcc := newTestRemoteConfigClient(t)
	rc := &remoteConfig{client: rcc}
	i := &testInstaller{
		daemonImpl: newDaemon(rc, pm, &env.Env{RemoteUpdates: true}),
		rcc:        rcc,
		pm:         pm,
	}
	i.Start(context.Background())
	return i
}

func (i *testInstaller) Stop() {
	i.daemonImpl.Stop(context.Background())
}

func TestInstall(t *testing.T) {
	i := newTestInstaller(t)
	defer i.Stop()

	testURL := "oci://example.com/test-package:1.0.0"
	i.pm.On("Install", mock.Anything, testURL, []string(nil)).Return(nil).Once()

	err := i.Install(context.Background(), testURL, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	i.pm.AssertExpectations(t)
}

func TestStartExperiment(t *testing.T) {
	i := newTestInstaller(t)
	defer i.Stop()

	testURL := "oci://example.com/test-package:1.0.0"
	i.pm.On("InstallExperiment", mock.Anything, testURL).Return(nil).Once()

	err := i.StartExperiment(context.Background(), testURL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	i.pm.AssertExpectations(t)
}

func TestStopExperiment(t *testing.T) {
	i := newTestInstaller(t)
	defer i.Stop()

	pkg := "test-package"
	i.pm.On("RemoveExperiment", mock.Anything, pkg).Return(nil).Once()

	err := i.StopExperiment(context.Background(), pkg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	i.pm.AssertExpectations(t)
}

func TestPromoteExperiment(t *testing.T) {
	i := newTestInstaller(t)
	defer i.Stop()

	pkg := "test-package"
	i.pm.On("PromoteExperiment", mock.Anything, pkg).Return(nil).Once()

	err := i.PromoteExperiment(context.Background(), pkg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	i.pm.AssertExpectations(t)
}

func TestUpdateCatalog(t *testing.T) {
	i := newTestInstaller(t)
	defer i.Stop()

	testPackage := Package{
		Name:     "test-package",
		Version:  "1.0.0",
		URL:      "oci://example.com/test-package@sha256:2fa082d512a120a814e32ddb80454efce56595b5c84a37cc1a9f90cf9cc7ba85",
		Platform: runtime.GOOS,
		Arch:     runtime.GOARCH,
	}
	c := catalog{
		Packages: []Package{testPackage},
	}
	i.rcc.SubmitCatalog(c)
	pkg, err := i.daemonImpl.GetPackage("test-package", "1.0.0")

	assert.NoError(t, err)
	assert.Equal(t, testPackage, pkg)
	assert.Equal(t, c, i.daemonImpl.catalog)
	i.pm.AssertExpectations(t)
}

func TestRemoteRequest(t *testing.T) {
	i := newTestInstaller(t)
	defer i.Stop()

	testStablePackage := Package{
		Name:    "test-package",
		Version: "0.0.1",
	}
	testExperimentPackage := Package{
		Name:     "test-package",
		Version:  "1.0.0",
		URL:      "oci://example.com/test-package@sha256:5e884898da28047151d0e56f8dc6292773603d0d6aabbdd62a11ef721d1542d8",
		Platform: runtime.GOOS,
		Arch:     runtime.GOARCH,
	}
	c := catalog{
		Packages: []Package{testExperimentPackage},
	}
	versionParams := taskWithVersionParams{
		Version: testExperimentPackage.Version,
	}
	versionParamsJSON, _ := json.Marshal(versionParams)
	i.rcc.SubmitCatalog(c)

	testRequest := remoteAPIRequest{
		ID:            "test-request-1",
		Method:        methodStartExperiment,
		Package:       testExperimentPackage.Name,
		ExpectedState: expectedState{InstallerVersion: version.AgentVersion, Stable: testStablePackage.Version},
		Params:        versionParamsJSON,
	}
	i.pm.On("State", testStablePackage.Name).Return(repository.State{Stable: testStablePackage.Version}, nil).Once()
	i.pm.On("InstallExperiment", mock.Anything, testExperimentPackage.URL).Return(nil).Once()
	i.rcc.SubmitRequest(testRequest)
	i.requestsWG.Wait()

	testRequest = remoteAPIRequest{
		ID:            "test-request-2",
		Method:        methodStopExperiment,
		Package:       testExperimentPackage.Name,
		ExpectedState: expectedState{InstallerVersion: version.AgentVersion, Stable: testStablePackage.Version, Experiment: testExperimentPackage.Version},
	}
	i.pm.On("State", testStablePackage.Name).Return(repository.State{Stable: testStablePackage.Version, Experiment: testExperimentPackage.Version}, nil).Once()
	i.pm.On("RemoveExperiment", mock.Anything, testExperimentPackage.Name).Return(nil).Once()
	i.rcc.SubmitRequest(testRequest)
	i.requestsWG.Wait()

	testRequest = remoteAPIRequest{
		ID:            "test-request-3",
		Method:        methodPromoteExperiment,
		Package:       testExperimentPackage.Name,
		ExpectedState: expectedState{InstallerVersion: version.AgentVersion, Stable: testStablePackage.Version, Experiment: testExperimentPackage.Version},
	}
	i.pm.On("State", testStablePackage.Name).Return(repository.State{Stable: testStablePackage.Version, Experiment: testExperimentPackage.Version}, nil).Once()
	i.pm.On("PromoteExperiment", mock.Anything, testExperimentPackage.Name).Return(nil).Once()
	i.rcc.SubmitRequest(testRequest)
	i.requestsWG.Wait()

	i.pm.AssertExpectations(t)
}
