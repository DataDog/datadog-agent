// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// for now the installer is not supported on windows
//go:build !windows

package daemon

import (
	"context"
	"net"
	"net/http"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/repository"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type testInstallerMock struct {
	mock.Mock
}

func (m *testInstallerMock) Start(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *testInstallerMock) Stop(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *testInstallerMock) Install(ctx context.Context, url string) error {
	args := m.Called(ctx, url)
	return args.Error(0)
}

func (m *testInstallerMock) StartExperiment(ctx context.Context, url string) error {
	args := m.Called(ctx, url)
	return args.Error(0)
}

func (m *testInstallerMock) StopExperiment(ctx context.Context, pkg string) error {
	args := m.Called(ctx, pkg)
	return args.Error(0)
}

func (m *testInstallerMock) PromoteExperiment(ctx context.Context, pkg string) error {
	args := m.Called(ctx, pkg)
	return args.Error(0)
}

func (m *testInstallerMock) GetPackage(pkg string, version string) (Package, error) {
	args := m.Called(pkg, version)
	return args.Get(0).(Package), args.Error(1)
}

func (m *testInstallerMock) GetState() (map[string]repository.State, error) {
	args := m.Called()
	return args.Get(0).(map[string]repository.State), args.Error(1)
}

type testLocalAPI struct {
	i *testInstallerMock
	s *localAPIImpl
	c *localAPIClientImpl
}

func newTestLocalAPI(t *testing.T) *testLocalAPI {
	installer := &testInstallerMock{}
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
	return &testLocalAPI{installer, apiServer, apiClient}
}

func (api *testLocalAPI) Stop() {
	api.s.Stop(context.Background())
}

func TestAPIStatus(t *testing.T) {
	api := newTestLocalAPI(t)
	defer api.Stop()

	installerState := map[string]repository.State{
		"pkg1": {
			Stable:     "1.0.0",
			Experiment: "2.0.0",
		},
	}
	api.i.On("GetState").Return(installerState, nil)

	resp, err := api.c.Status()

	assert.NoError(t, err)
	assert.Nil(t, resp.Error)
	assert.Equal(t, version.AgentVersion, resp.Version)
	assert.Equal(t, installerState, resp.Packages)
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
	api.i.On("Install", mock.Anything, testPackage.URL).Return(nil)

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
