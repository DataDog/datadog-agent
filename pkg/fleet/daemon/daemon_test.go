// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package daemon

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/nacl/box"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/config"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/repository"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/version"
)

type testBoostrapper struct {
	mock.Mock
}

func (b *testBoostrapper) InstallExperiment(ctx context.Context, env *env.Env, url string) error {
	args := b.Called(ctx, env, url)
	return args.Error(0)
}

type testPackageManager struct {
	mock.Mock
}

func (m *testPackageManager) IsInstalled(ctx context.Context, pkg string) (bool, error) {
	args := m.Called(ctx, pkg)
	return args.Bool(0), args.Error(1)
}

func (m *testPackageManager) AvailableDiskSpace() (uint64, error) {
	args := m.Called()
	return args.Get(0).(uint64), args.Error(1)
}

func (m *testPackageManager) State(ctx context.Context, pkg string) (repository.State, error) {
	args := m.Called(ctx, pkg)
	return args.Get(0).(repository.State), args.Error(1)
}

func (m *testPackageManager) ConfigState(ctx context.Context, pkg string) (repository.State, error) {
	args := m.Called(ctx, pkg)
	return args.Get(0).(repository.State), args.Error(1)
}

func (m *testPackageManager) ConfigAndPackageStates(ctx context.Context) (*repository.PackageStates, error) {
	args := m.Called(ctx)
	return args.Get(0).(*repository.PackageStates), args.Error(1)
}

func (m *testPackageManager) Install(ctx context.Context, url string, installArgs []string) error {
	args := m.Called(ctx, url, installArgs)
	return args.Error(0)
}

func (m *testPackageManager) ForceInstall(ctx context.Context, url string, installArgs []string) error {
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

func (m *testPackageManager) InstallConfigExperiment(ctx context.Context, pkg string, operations config.Operations, decryptedSecrets map[string]string) error {
	args := m.Called(ctx, pkg, operations, decryptedSecrets)
	return args.Error(0)
}

func (m *testPackageManager) RemoveConfigExperiment(ctx context.Context, pkg string) error {
	args := m.Called(ctx, pkg)
	return args.Error(0)
}

func (m *testPackageManager) PromoteConfigExperiment(ctx context.Context, pkg string) error {
	args := m.Called(ctx, pkg)
	return args.Error(0)
}

func (m *testPackageManager) GarbageCollect(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *testPackageManager) SetupInstaller(ctx context.Context, path string) error {
	args := m.Called(ctx, path)
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

func (m *testPackageManager) InstallExtensions(ctx context.Context, url string, extensions []string) error {
	args := m.Called(ctx, url, extensions)
	return args.Error(0)
}

func (m *testPackageManager) RemoveExtensions(ctx context.Context, pkg string, extensions []string) error {
	args := m.Called(ctx, pkg, extensions)
	return args.Error(0)
}

func (m *testPackageManager) SaveExtensions(ctx context.Context, pkg string, path string) error {
	args := m.Called(ctx, pkg, path)
	return args.Error(0)
}

func (m *testPackageManager) RestoreExtensions(ctx context.Context, url string, path string) error {
	args := m.Called(ctx, url, path)
	return args.Error(0)
}

func (m *testPackageManager) Close() error {
	args := m.Called()
	return args.Error(0)
}

type testRemoteConfigClient struct {
	sync.Mutex
	t              *testing.T
	clientID       string
	listeners      map[string][]func(map[string]state.RawConfig, func(cfgPath string, status state.ApplyStatus))
	installerState *pbgo.ClientUpdater
}

func newTestRemoteConfigClient(t *testing.T) *testRemoteConfigClient {
	return &testRemoteConfigClient{
		t:         t,
		clientID:  "test-client-id",
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

func (c *testRemoteConfigClient) SetInstallerState(state *pbgo.ClientUpdater) {
	c.Lock()
	defer c.Unlock()
	c.installerState = state
}

func (c *testRemoteConfigClient) GetInstallerState() *pbgo.ClientUpdater {
	c.Lock()
	defer c.Unlock()
	return c.installerState
}

func (c *testRemoteConfigClient) GetClientID() string {
	return c.clientID
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
	bm  *testBoostrapper
}

func newTestInstaller(t *testing.T) *testInstaller {
	bm := &testBoostrapper{}
	installExperimentFunc = bm.InstallExperiment
	pm := &testPackageManager{}
	pm.On("AvailableDiskSpace").Return(uint64(1000000000), nil)
	pm.On("ConfigAndPackageStates", mock.Anything).Return(&repository.PackageStates{
		States:       map[string]repository.State{},
		ConfigStates: map[string]repository.State{},
	}, nil)
	rcc := newTestRemoteConfigClient(t)
	rc := &remoteConfig{client: rcc}
	taskDB, err := newTaskDB(filepath.Join(t.TempDir(), "tasks.db"))
	require.NoError(t, err)
	secretsPubKey, secretsPrivKey, err := box.GenerateKey(rand.Reader)
	require.NoError(t, err)

	daemon := newDaemon(
		rc,
		func(_ *env.Env) installer.Installer { return pm },
		&env.Env{RemoteUpdates: true},
		taskDB,
		30*time.Second,
		1*time.Hour,
		secretsPubKey,
		secretsPrivKey,
	)
	i := &testInstaller{
		daemonImpl: daemon,
		rcc:        rcc,
		pm:         pm,
		bm:         bm,
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
	i.bm.On("InstallExperiment", mock.Anything, mock.Anything, testURL).Return(nil).Once()

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
	versionParams := experimentTaskParams{
		Version: testExperimentPackage.Version,
	}
	versionParamsJSON, _ := json.Marshal(versionParams)
	i.rcc.SubmitCatalog(c)

	testRequest := remoteAPIRequest{
		ID:            "test-request-1",
		Method:        methodStartExperiment,
		Package:       testExperimentPackage.Name,
		ExpectedState: expectedState{InstallerVersion: version.AgentVersion, Stable: testStablePackage.Version, StableConfig: testStablePackage.Version, ClientID: i.rcc.GetClientID()},
		Params:        versionParamsJSON,
	}
	i.pm.On("State", mock.Anything, testStablePackage.Name).Return(repository.State{Stable: testStablePackage.Version}, nil).Once()
	i.pm.On("ConfigState", mock.Anything, testStablePackage.Name).Return(repository.State{Stable: testStablePackage.Version}, nil).Once()
	i.bm.On("InstallExperiment", mock.Anything, mock.Anything, testExperimentPackage.URL).Return(nil).Once()
	i.rcc.SubmitRequest(testRequest)
	i.requestsWG.Wait()

	testRequest = remoteAPIRequest{
		ID:            "test-request-2",
		Method:        methodStopExperiment,
		Package:       testExperimentPackage.Name,
		ExpectedState: expectedState{InstallerVersion: version.AgentVersion, Stable: testStablePackage.Version, Experiment: testExperimentPackage.Version, StableConfig: testStablePackage.Version, ClientID: i.rcc.GetClientID()},
	}
	i.pm.On("State", mock.Anything, testStablePackage.Name).Return(repository.State{Stable: testStablePackage.Version, Experiment: testExperimentPackage.Version}, nil).Once()
	i.pm.On("ConfigState", mock.Anything, testStablePackage.Name).Return(repository.State{Stable: testStablePackage.Version}, nil).Once()
	i.pm.On("RemoveExperiment", mock.Anything, testExperimentPackage.Name).Return(nil).Once()
	i.rcc.SubmitRequest(testRequest)
	i.requestsWG.Wait()

	testRequest = remoteAPIRequest{
		ID:            "test-request-3",
		Method:        methodPromoteExperiment,
		Package:       testExperimentPackage.Name,
		ExpectedState: expectedState{InstallerVersion: version.AgentVersion, Stable: testStablePackage.Version, Experiment: testExperimentPackage.Version, StableConfig: testStablePackage.Version, ClientID: i.rcc.GetClientID()},
	}
	i.pm.On("State", mock.Anything, testStablePackage.Name).Return(repository.State{Stable: testStablePackage.Version, Experiment: testExperimentPackage.Version}, nil).Once()
	i.pm.On("ConfigState", mock.Anything, testStablePackage.Name).Return(repository.State{Stable: testStablePackage.Version}, nil).Once()
	i.pm.On("PromoteExperiment", mock.Anything, testExperimentPackage.Name).Return(nil).Once()
	i.rcc.SubmitRequest(testRequest)
	i.requestsWG.Wait()

	i.pm.AssertExpectations(t)
}

func TestRemoteRequestClientIDCheckDisabled(t *testing.T) {
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
	versionParams := experimentTaskParams{
		Version: testExperimentPackage.Version,
	}
	versionParamsJSON, _ := json.Marshal(versionParams)
	i.rcc.SubmitCatalog(c)

	// Submit a request with the special disableClientIDCheck value
	testRequest := remoteAPIRequest{
		ID:            "test-request-disable-check",
		Method:        methodStartExperiment,
		Package:       testExperimentPackage.Name,
		ExpectedState: expectedState{InstallerVersion: version.AgentVersion, Stable: testStablePackage.Version, StableConfig: testStablePackage.Version, ClientID: disableClientIDCheck},
		Params:        versionParamsJSON,
	}
	i.pm.On("State", mock.Anything, testStablePackage.Name).Return(repository.State{Stable: testStablePackage.Version}, nil).Once()
	i.pm.On("ConfigState", mock.Anything, testStablePackage.Name).Return(repository.State{Stable: testStablePackage.Version}, nil).Once()
	i.bm.On("InstallExperiment", mock.Anything, mock.Anything, testExperimentPackage.URL).Return(nil).Once()
	i.rcc.SubmitRequest(testRequest)
	i.requestsWG.Wait()

	// Verify that InstallExperiment was called even though client ID is the special bypass value
	i.bm.AssertExpectations(t)
	i.pm.AssertExpectations(t)
}

func TestRefreshStateRunningVersions(t *testing.T) {
	// Setup test state
	testPackageStates := map[string]repository.State{
		"datadog-agent": {
			Stable:     "7.50.0",
			Experiment: "7.51.0",
		},
	}
	testConfigStates := map[string]repository.State{
		"datadog-agent": {
			Stable:     "config-stable-1",
			Experiment: "config-exp-1",
		},
	}

	// Create test components with our custom mocks
	bm := &testBoostrapper{}
	installExperimentFunc = bm.InstallExperiment
	pm := &testPackageManager{}
	pm.On("AvailableDiskSpace").Return(uint64(1000000000), nil)
	pm.On("ConfigAndPackageStates", mock.Anything).Return(&repository.PackageStates{
		States:       testPackageStates,
		ConfigStates: testConfigStates,
	}, nil)
	rcc := newTestRemoteConfigClient(t)
	rc := &remoteConfig{client: rcc}
	taskDB, err := newTaskDB(filepath.Join(t.TempDir(), "tasks.db"))
	require.NoError(t, err)
	secretsPubKey, secretsPrivKey, err := box.GenerateKey(rand.Reader)
	require.NoError(t, err)

	testEnv := &env.Env{
		RemoteUpdates: true,
		ConfigID:      "test-config-id-123",
	}
	daemon := newDaemon(
		rc,
		func(_ *env.Env) installer.Installer { return pm },
		testEnv,
		taskDB,
		30*time.Second,
		1*time.Hour,
		secretsPubKey,
		secretsPrivKey,
	)
	i := &testInstaller{
		daemonImpl: daemon,
		rcc:        rcc,
		pm:         pm,
		bm:         bm,
	}
	i.Start(context.Background())
	defer i.Stop()

	// Call refreshState to trigger the state update
	i.daemonImpl.refreshState(context.Background())

	// Wait a bit for the state to be set
	require.Eventually(t, func() bool {
		state := i.rcc.GetInstallerState()
		return state != nil && len(state.Packages) > 0
	}, 1*time.Second, 10*time.Millisecond)

	// Get the state and verify RunningVersion and RunningConfigVersion are set correctly
	state := i.rcc.GetInstallerState()
	require.NotNil(t, state)
	require.Len(t, state.Packages, 1)

	pkg := state.Packages[0]
	assert.Equal(t, "datadog-agent", pkg.Package)
	assert.Equal(t, "7.50.0", pkg.StableVersion)
	assert.Equal(t, "7.51.0", pkg.ExperimentVersion)
	assert.Equal(t, "config-stable-1", pkg.StableConfigVersion)
	assert.Equal(t, "config-exp-1", pkg.ExperimentConfigVersion)
	assert.Equal(t, version.AgentPackageVersion, pkg.RunningVersion, "RunningVersion should be set to AgentPackageVersion")
	assert.Equal(t, "test-config-id-123", pkg.RunningConfigVersion, "RunningConfigVersion should be set to env.ConfigID")
	assert.Equal(t, state.SecretsPubKey, base64.StdEncoding.EncodeToString(secretsPubKey[:]))

	pm.AssertExpectations(t)
}

func TestStopDoesNotDeadlockWithConcurrentRequest(t *testing.T) {
	i := newTestInstaller(t)

	// Simulate a request that has been scheduled (requestsWG.Add called) but whose
	// handler has not yet acquired the mutex. handleRemoteAPIRequest defers both
	// d.m.Unlock() and d.requestsWG.Done(), so Done() can only be called once the
	// mutex is acquired. If Stop() holds the mutex while calling requestsWG.Wait(),
	// both goroutines deadlock.
	i.daemonImpl.requestsWG.Add(1)

	stopDone := make(chan error, 1)
	go func() {
		stopDone <- i.daemonImpl.Stop(context.Background())
	}()

	// Simulate handleRemoteAPIRequest competing for the mutex to call Done().
	// A small sleep increases the likelihood that Stop() acquires the mutex first,
	// which is the scenario that deadlocks in the old code.
	go func() {
		time.Sleep(10 * time.Millisecond)
		i.daemonImpl.m.Lock()
		i.daemonImpl.requestsWG.Done()
		i.daemonImpl.m.Unlock()
	}()

	select {
	case err := <-stopDone:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Stop() deadlocked: timed out after 5 seconds")
	}
}

func TestDecryptSecrets(t *testing.T) {
	// Generate test encryption keys
	pubKey, privKey, err := box.GenerateKey(rand.Reader)
	require.NoError(t, err)

	// Helper function to encrypt and encode a secret
	encryptSecret := func(t *testing.T, plaintext string, pubKey *[32]byte) string {
		encrypted, err := box.SealAnonymous(nil, []byte(plaintext), pubKey, rand.Reader)
		require.NoError(t, err)
		return base64.StdEncoding.EncodeToString(encrypted)
	}

	t.Run("successfully decrypt secrets and skip unreferenced", func(t *testing.T) {
		d := &daemonImpl{
			secretsPubKey:  pubKey,
			secretsPrivKey: privKey,
		}

		apiKey := "my-api-key"
		appKey := "my-app-key"
		encryptedAPI := encryptSecret(t, apiKey, pubKey)
		encryptedApp := encryptSecret(t, appKey, pubKey)
		encryptedUnused := encryptSecret(t, "unused", pubKey)

		ops := config.Operations{
			DeploymentID: "test-config",
			FileOperations: []config.FileOperation{
				{
					Patch: []byte(`api_key: SEC[apikey]`),
				},
				{
					Patch: []byte(`app_key: SEC[appkey]`),
				},
			},
		}

		decryptedSecrets, err := d.decryptSecrets(ops, map[string]string{
			"apikey": encryptedAPI,
			"appkey": encryptedApp,
			"unused": encryptedUnused,
		})

		require.NoError(t, err)
		assert.Equal(t, apiKey, decryptedSecrets["apikey"])
		assert.Equal(t, appKey, decryptedSecrets["appkey"])
		assert.NotContains(t, decryptedSecrets, "unused")
		assert.Len(t, decryptedSecrets, 2)
	})

	t.Run("empty secrets map", func(t *testing.T) {
		d := &daemonImpl{
			secretsPubKey:  pubKey,
			secretsPrivKey: privKey,
		}

		ops := config.Operations{
			DeploymentID: "test-config",
			FileOperations: []config.FileOperation{
				{
					Patch: []byte(`log_level: debug`),
				},
			},
		}

		decryptedSecrets, err := d.decryptSecrets(ops, map[string]string{})

		require.NoError(t, err)
		assert.Empty(t, decryptedSecrets)
	})

	t.Run("decryption errors", func(t *testing.T) {
		d := &daemonImpl{
			secretsPubKey:  pubKey,
			secretsPrivKey: privKey,
		}

		ops := config.Operations{
			DeploymentID: "test-config",
			FileOperations: []config.FileOperation{
				{
					Patch: []byte(`api_key: SEC[apikey]`),
				},
			},
		}

		// Invalid base64
		_, err := d.decryptSecrets(ops, map[string]string{
			"apikey": "not-valid-base64!!!",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "could not decode secret")

		// Wrong encryption key
		wrongPubKey, _, err := box.GenerateKey(rand.Reader)
		require.NoError(t, err)
		encryptedWithWrongKey := encryptSecret(t, "secret", wrongPubKey)

		_, err = d.decryptSecrets(ops, map[string]string{
			"apikey": encryptedWithWrongKey,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "could not decrypt secret")
	})
}
