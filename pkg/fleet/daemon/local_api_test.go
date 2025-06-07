// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package daemon

import (
	"context"
	"net"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

type testDaemon struct {
	mock.Mock
}

func (m *testDaemon) Start(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *testDaemon) Stop(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *testDaemon) Install(ctx context.Context, url string, installArgs []string) error {
	args := m.Called(ctx, url, installArgs)
	return args.Error(0)
}

func (m *testDaemon) Remove(ctx context.Context, pkg string) error {
	args := m.Called(ctx, pkg)
	return args.Error(0)
}

func (m *testDaemon) StartExperiment(ctx context.Context, url string) error {
	args := m.Called(ctx, url)
	return args.Error(0)
}

func (m *testDaemon) StopExperiment(ctx context.Context, pkg string) error {
	args := m.Called(ctx, pkg)
	return args.Error(0)
}

func (m *testDaemon) PromoteExperiment(ctx context.Context, pkg string) error {
	args := m.Called(ctx, pkg)
	return args.Error(0)
}

func (m *testDaemon) StartConfigExperiment(ctx context.Context, url string, hash string) error {
	args := m.Called(ctx, url, hash)
	return args.Error(0)
}

func (m *testDaemon) StopConfigExperiment(ctx context.Context, pkg string) error {
	args := m.Called(ctx, pkg)
	return args.Error(0)
}

func (m *testDaemon) PromoteConfigExperiment(ctx context.Context, pkg string) error {
	args := m.Called(ctx, pkg)
	return args.Error(0)
}

func (m *testDaemon) GetPackage(pkg string, version string) (Package, error) {
	args := m.Called(pkg, version)
	return args.Get(0).(Package), args.Error(1)
}

func (m *testDaemon) GetState(ctx context.Context) (map[string]PackageState, error) {
	args := m.Called(ctx)
	return args.Get(0).(map[string]PackageState), args.Error(1)
}

func (m *testDaemon) GetRemoteConfigState() *pbgo.ClientUpdater {
	args := m.Called()
	return args.Get(0).(*pbgo.ClientUpdater)
}

func (m *testDaemon) GetAPMInjectionStatus() (APMInjectionStatus, error) {
	args := m.Called()
	return args.Get(0).(APMInjectionStatus), args.Error(1)
}

func (m *testDaemon) SetCatalog(catalog catalog) {
	m.Called(catalog)
}

func (m *testDaemon) SetConfigCatalog(configs map[string]installerConfig) {
	m.Called(configs)
}

type testLocalAPI struct {
	i *testDaemon
	s *localAPIImpl
	c *localAPIClientImpl
}

func newTestLocalAPI(t *testing.T) *testLocalAPI {
	daemon := &testDaemon{}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	apiServer := &localAPIImpl{
		server:   &http.Server{},
		listener: l,
		daemon:   daemon,
	}
	apiServer.Start(context.Background())
	apiClient := &localAPIClientImpl{
		client: &http.Client{},
		addr:   l.Addr().String(),
	}
	return &testLocalAPI{daemon, apiServer, apiClient}
}

func (api *testLocalAPI) Stop() {
	api.s.Stop(context.Background())
}

func TestAPIStatus(t *testing.T) {
	api := newTestLocalAPI(t)
	defer api.Stop()

	remoteConfigState := &pbgo.ClientUpdater{
		Packages: []*pbgo.PackageState{
			{
				Package: "test-package",
				Task: &pbgo.PackageStateTask{
					State: pbgo.TaskState_DONE,
				},
			},
		},
	}
	api.i.On("GetRemoteConfigState").Return(remoteConfigState, nil)

	resp, err := api.c.Status()

	assert.NoError(t, err)
	assert.Nil(t, resp.Error)
	assert.Equal(t, resp.RemoteConfigState, remoteConfigState.Packages)
}

func TestAPIInstall(t *testing.T) {
	api := newTestLocalAPI(t)
	defer api.Stop()

	testPackage := Package{
		Name:    "test-package",
		Version: "1.0.0",
		URL:     "oci://example.com/test-package@5e884898da28047151d0e56f8dc6292773603d0d6aabbdd62a11ef721d1542d8",
	}
	api.i.On("GetPackage", testPackage.Name, testPackage.Version).Return(testPackage, nil)
	api.i.On("Install", mock.Anything, testPackage.URL, []string(nil)).Return(nil)

	err := api.c.Install(testPackage.Name, testPackage.Version)

	assert.NoError(t, err)
}

func TestAPIStartExperiment(t *testing.T) {
	api := newTestLocalAPI(t)
	defer api.Stop()

	testPackage := Package{
		Name:    "test-package",
		Version: "1.0.0",
		URL:     "oci://example.com/test-package@5e884898da28047151d0e56f8dc6292773603d0d6aabbdd62a11ef721d1542d8",
	}
	api.i.On("GetPackage", testPackage.Name, testPackage.Version).Return(testPackage, nil)
	api.i.On("StartExperiment", mock.Anything, testPackage.URL).Return(nil)

	err := api.c.StartExperiment(testPackage.Name, testPackage.Version)

	assert.NoError(t, err)
}

func TestAPIStopExperiment(t *testing.T) {
	api := newTestLocalAPI(t)
	defer api.Stop()

	testPackage := "test-package"
	api.i.On("StopExperiment", mock.Anything, testPackage).Return(nil)

	err := api.c.StopExperiment(testPackage)

	assert.NoError(t, err)
}

func TestAPIPromoteExperiment(t *testing.T) {
	api := newTestLocalAPI(t)
	defer api.Stop()

	testPackage := "test-package"
	api.i.On("PromoteExperiment", mock.Anything, testPackage).Return(nil)

	err := api.c.PromoteExperiment(testPackage)

	assert.NoError(t, err)
}
