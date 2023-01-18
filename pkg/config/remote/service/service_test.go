// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package service

import (
	"context"
	"encoding/base32"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/DataDog/go-tuf/data"
	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/DataDog/datadog-agent/pkg/config"
	rdata "github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/config/remote/uptane"
	"github.com/DataDog/datadog-agent/pkg/proto/msgpgo"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/DataDog/datadog-agent/pkg/version"
)

type mockAPI struct {
	mock.Mock
}

const (
	httpError = "api: simulated HTTP error"
)

func (m *mockAPI) Fetch(ctx context.Context, request *pbgo.LatestConfigsRequest) (*pbgo.LatestConfigsResponse, error) {
	args := m.Called(ctx, request)
	return args.Get(0).(*pbgo.LatestConfigsResponse), args.Error(1)
}

func (m *mockAPI) FetchOrgData(ctx context.Context) (*pbgo.OrgDataResponse, error) {
	args := m.Called(ctx)
	return args.Get(0).(*pbgo.OrgDataResponse), args.Error(1)
}

type mockUptane struct {
	mock.Mock
}

func (m *mockUptane) Update(response *pbgo.LatestConfigsResponse) error {
	args := m.Called(response)
	return args.Error(0)
}

func (m *mockUptane) State() (uptane.State, error) {
	args := m.Called()
	return args.Get(0).(uptane.State), args.Error(1)
}

func (m *mockUptane) DirectorRoot(version uint64) ([]byte, error) {
	args := m.Called(version)
	return args.Get(0).([]byte), args.Error(1)
}

func (m *mockUptane) StoredOrgUUID() (string, error) {
	args := m.Called()
	return args.Get(0).(string), args.Error(1)
}

func (m *mockUptane) Targets() (data.TargetFiles, error) {
	args := m.Called()
	return args.Get(0).(data.TargetFiles), args.Error(1)
}

func (m *mockUptane) TargetFile(path string) ([]byte, error) {
	args := m.Called(path)
	return args.Get(0).([]byte), args.Error(1)
}

func (m *mockUptane) TargetsMeta() ([]byte, error) {
	args := m.Called()
	return args.Get(0).([]byte), args.Error(1)
}

func (m *mockUptane) TargetsCustom() ([]byte, error) {
	args := m.Called()
	return args.Get(0).([]byte), args.Error(1)
}

func (m *mockUptane) TUFVersionState() (uptane.TUFVersions, error) {
	args := m.Called()
	return args.Get(0).(uptane.TUFVersions), args.Error(1)
}

var testRCKey = msgpgo.RemoteConfigKey{
	AppKey:     "fake_key",
	OrgID:      2,
	Datacenter: "dd.com",
}

func newTestService(t *testing.T, api *mockAPI, uptane *mockUptane, clock clock.Clock) *Service {
	config.Datadog.Set("hostname", "test-hostname")
	defer config.Datadog.Set("hostname", "")

	dir, err := os.MkdirTemp("", "testdbdir")
	assert.NoError(t, err)
	config.Datadog.Set("run_path", dir)
	serializedKey, _ := testRCKey.MarshalMsg(nil)
	config.Datadog.Set("remote_configuration.key", base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(serializedKey))
	service, err := NewService()
	assert.NoError(t, err)
	service.api = api
	service.clock = clock
	service.uptane = uptane
	assert.NoError(t, err)
	return service
}

func TestServiceBackoffFailure(t *testing.T) {
	api := &mockAPI{}
	uptaneClient := &mockUptane{}
	clock := clock.NewMock()
	service := newTestService(t, api, uptaneClient, clock)

	lastConfigResponse := &pbgo.LatestConfigsResponse{
		TargetFiles: []*pbgo.File{{Path: "test"}},
	}

	api.On("Fetch", mock.Anything, &pbgo.LatestConfigsRequest{
		Hostname:                     service.hostname,
		AgentVersion:                 version.AgentVersion,
		CurrentConfigSnapshotVersion: 0,
		CurrentConfigRootVersion:     0,
		CurrentDirectorRootVersion:   0,
		Products:                     []string{},
		NewProducts:                  []string{},
		OrgUuid:                      "abcdef",
	}).Return(lastConfigResponse, errors.New("simulated HTTP error"))
	uptaneClient.On("StoredOrgUUID").Return("abcdef", nil)
	uptaneClient.On("TUFVersionState").Return(uptane.TUFVersions{}, nil)
	uptaneClient.On("Update", lastConfigResponse).Return(nil)
	uptaneClient.On("TargetsCustom").Return([]byte{}, nil)

	// We'll set the default interal to 1 second to make math less hard
	service.defaultRefreshInterval = 1 * time.Second

	// There should be no errors at the start
	assert.Equal(t, service.backoffErrorCount, 0)

	err := service.refresh()
	assert.NotNil(t, err)

	// Sending the http error too
	api.On("Fetch", mock.Anything, &pbgo.LatestConfigsRequest{
		Hostname:                     service.hostname,
		AgentVersion:                 version.AgentVersion,
		CurrentConfigSnapshotVersion: 0,
		CurrentConfigRootVersion:     0,
		CurrentDirectorRootVersion:   0,
		Products:                     []string{},
		NewProducts:                  []string{},
		HasError:                     true,
		Error:                        httpError,
		OrgUuid:                      "abcdef",
	}).Return(lastConfigResponse, errors.New("simulated HTTP error"))
	uptaneClient.On("StoredOrgUUID").Return("abcdef", nil)
	uptaneClient.On("TUFVersionState").Return(uptane.TUFVersions{}, nil)
	uptaneClient.On("Update", lastConfigResponse).Return(nil)
	uptaneClient.On("TargetsCustom").Return([]byte{}, nil)

	// We should be tracking an error now. With the default backoff config, our refresh interval
	// should be somewhere in the range of 1 + [30,60], so [31,61]
	assert.Equal(t, service.backoffErrorCount, 1)
	refreshInterval := service.calculateRefreshInterval()
	assert.GreaterOrEqual(t, refreshInterval, 31*time.Second)
	assert.LessOrEqual(t, refreshInterval, 61*time.Second)

	err = service.refresh()
	assert.NotNil(t, err)

	// Now we're looking at  1 + [60, 120], so [61,121]
	assert.Equal(t, service.backoffErrorCount, 2)
	refreshInterval = service.calculateRefreshInterval()
	assert.GreaterOrEqual(t, refreshInterval, 61*time.Second)
	assert.LessOrEqual(t, refreshInterval, 121*time.Second)

	err = service.refresh()
	assert.NotNil(t, err)

	// After one more we're looking at  1 + [120, 240], so [121,241]
	assert.Equal(t, service.backoffErrorCount, 3)
	refreshInterval = service.calculateRefreshInterval()
	assert.GreaterOrEqual(t, refreshInterval, 121*time.Second)
	assert.LessOrEqual(t, refreshInterval, 241*time.Second)
}

func TestServiceBackoffFailureRecovery(t *testing.T) {
	api := &mockAPI{}
	uptaneClient := &mockUptane{}
	clock := clock.NewMock()
	service := newTestService(t, api, uptaneClient, clock)

	lastConfigResponse := &pbgo.LatestConfigsResponse{
		TargetFiles: []*pbgo.File{{Path: "test"}},
	}

	api = &mockAPI{}
	api.On("Fetch", mock.Anything, &pbgo.LatestConfigsRequest{
		Hostname:                     service.hostname,
		AgentVersion:                 version.AgentVersion,
		CurrentConfigSnapshotVersion: 0,
		CurrentConfigRootVersion:     0,
		CurrentDirectorRootVersion:   0,
		Products:                     []string{},
		NewProducts:                  []string{},
		OrgUuid:                      "abcdef",
	}).Return(lastConfigResponse, nil)
	uptaneClient.On("StoredOrgUUID").Return("abcdef", nil)
	uptaneClient.On("TUFVersionState").Return(uptane.TUFVersions{}, nil)
	uptaneClient.On("Update", lastConfigResponse).Return(nil)
	uptaneClient.On("TargetsCustom").Return([]byte{}, nil)
	service.api = api

	// Artificially set the backoff error count so we can test recovery
	service.backoffErrorCount = 3

	// We'll set the default interal to 1 second to make math less hard
	service.defaultRefreshInterval = 1 * time.Second

	err := service.refresh()
	assert.Nil(t, err)

	// Our recovery interval is 2, so we should step back to the [31,61] range
	assert.Equal(t, service.backoffErrorCount, 1)
	refreshInterval := service.calculateRefreshInterval()
	assert.GreaterOrEqual(t, refreshInterval, 31*time.Second)
	assert.LessOrEqual(t, refreshInterval, 61*time.Second)

	err = service.refresh()
	assert.Nil(t, err)

	// After a 2nd success, we'll be back to not having a backoff added.
	assert.Equal(t, service.backoffErrorCount, 0)
	refreshInterval = service.calculateRefreshInterval()
	assert.Equal(t, 1*time.Second, refreshInterval)
}

func customMeta(tracerPredicates []*pbgo.TracerPredicateV1, expiration int64) *json.RawMessage {
	data, err := json.Marshal(ConfigFileMetaCustom{Predicates: &pbgo.TracerPredicates{TracerPredicatesV1: tracerPredicates}, Expires: expiration})
	if err != nil {
		panic(err)
	}

	raw := json.RawMessage(data)
	return &raw
}

// TestClientGetConfigsRequestMissingState sets up just enough of a mock server to validate
// that if a client request does NOT include the Client object or the
// Client.State object the request results in an error equivalent to
// gRPC's InvalidArgument status code.
func TestClientGetConfigsRequestMissingFields(t *testing.T) {
	api := &mockAPI{}
	uptaneClient := &mockUptane{}
	clock := clock.NewMock()
	service := newTestService(t, api, uptaneClient, clock)

	uptaneClient.On("TUFVersionState").Return(uptane.TUFVersions{}, nil)

	// The Client object is missing
	req := &pbgo.ClientGetConfigsRequest{}
	_, err := service.ClientGetConfigs(context.Background(), req)
	assert.Error(t, err)
	assert.Equal(t, status.Convert(err).Code(), codes.InvalidArgument)

	// The Client object is present, but State is missing
	req.Client = &pbgo.Client{}
	_, err = service.ClientGetConfigs(context.Background(), req)
	assert.Error(t, err)
	assert.Equal(t, status.Convert(err).Code(), codes.InvalidArgument)

	// The Client object and State is present, but the root version indicates the client has no initial root
	req.Client = &pbgo.Client{
		State: &pbgo.ClientState{},
	}
	_, err = service.ClientGetConfigs(context.Background(), req)
	assert.Error(t, err)
	assert.Equal(t, status.Convert(err).Code(), codes.InvalidArgument)

	// The Client object and State is present but the client is an agent-client with no info
	req.Client = &pbgo.Client{
		State: &pbgo.ClientState{
			RootVersion: 1,
		},
		IsAgent: true,
	}
	_, err = service.ClientGetConfigs(context.Background(), req)
	assert.Error(t, err)
	assert.Equal(t, status.Convert(err).Code(), codes.InvalidArgument)

	// The Client object and State is present but the client is a tracer-client with no info
	req.Client = &pbgo.Client{
		State: &pbgo.ClientState{
			RootVersion: 1,
		},
		IsTracer: true,
	}
	_, err = service.ClientGetConfigs(context.Background(), req)
	assert.Error(t, err)
	assert.Equal(t, status.Convert(err).Code(), codes.InvalidArgument)

	// The client says its both a tracer and an agent
	req.Client = &pbgo.Client{
		State: &pbgo.ClientState{
			RootVersion: 1,
		},
		IsTracer:     true,
		ClientAgent:  &pbgo.ClientAgent{},
		IsAgent:      true,
		ClientTracer: &pbgo.ClientTracer{},
	}
	_, err = service.ClientGetConfigs(context.Background(), req)
	assert.Error(t, err)
	assert.Equal(t, status.Convert(err).Code(), codes.InvalidArgument)
}

func TestService(t *testing.T) {
	api := &mockAPI{}
	uptaneClient := &mockUptane{}
	clock := clock.NewMock()
	service := newTestService(t, api, uptaneClient, clock)

	lastConfigResponse := &pbgo.LatestConfigsResponse{
		TargetFiles: []*pbgo.File{{Path: "test"}},
	}
	api.On("Fetch", mock.Anything, &pbgo.LatestConfigsRequest{
		Hostname:                     service.hostname,
		AgentVersion:                 version.AgentVersion,
		CurrentConfigSnapshotVersion: 0,
		CurrentConfigRootVersion:     0,
		CurrentDirectorRootVersion:   0,
		Products:                     []string{},
		NewProducts:                  []string{},
		OrgUuid:                      "abcdef",
	}).Return(lastConfigResponse, nil)
	uptaneClient.On("StoredOrgUUID").Return("abcdef", nil)
	uptaneClient.On("TUFVersionState").Return(uptane.TUFVersions{}, nil)
	uptaneClient.On("Update", lastConfigResponse).Return(nil)
	uptaneClient.On("TargetsCustom").Return([]byte{}, nil)

	err := service.refresh()
	assert.NoError(t, err)

	api.AssertExpectations(t)
	uptaneClient.AssertExpectations(t)

	*uptaneClient = mockUptane{}
	*api = mockAPI{}

	root3 := []byte(`{"signatures": "testroot3", "signed": "signed"}`)
	canonicalRoot3 := []byte(`{"signatures":"testroot3","signed":"signed"}`)
	root4 := []byte(`{"signed": "signed", "signatures": "testroot4"}`)
	canonicalRoot4 := []byte(`{"signatures":"testroot4","signed":"signed"}`)
	targets := []byte(`{"signatures": "testtargets", "signed": "stuff"}`)
	canonicalTargets := []byte(`{"signatures":"testtargets","signed":"stuff"}`)
	testTargetsCustom := []byte(`{"opaque_backend_state":"dGVzdF9zdGF0ZQ=="}`)
	client := &pbgo.Client{
		Id: "testid",
		State: &pbgo.ClientState{
			RootVersion: 2,
		},
		IsAgent:     true,
		ClientAgent: &pbgo.ClientAgent{},
		Products: []string{
			string(rdata.ProductAPMSampling),
		},
	}
	fileAPM1 := []byte(`testapm1`)
	fileAPM2 := []byte(`testapm2`)
	uptaneClient.On("StoredOrgUUID").Return("abcdef", nil)
	uptaneClient.On("TargetsMeta").Return(targets, nil)
	uptaneClient.On("TargetsCustom").Return(testTargetsCustom, nil)

	uptaneClient.On("Targets").Return(data.TargetFiles{
		"datadog/2/APM_SAMPLING/id/1": {},
		"datadog/2/TESTING1/id/1":     {},
		"datadog/2/APM_SAMPLING/id/2": {},
		"datadog/2/APPSEC/id/1":       {},
	},
		nil,
	)
	uptaneClient.On("State").Return(uptane.State{
		ConfigState: map[string]uptane.MetaState{
			"root.json":      {Version: 1},
			"snapshot.json":  {Version: 2},
			"timestamp.json": {Version: 3},
			"targets.json":   {Version: 4},
			"role1.json":     {Version: 5},
		},
		DirectorState: map[string]uptane.MetaState{
			"root.json":    {Version: 4},
			"targets.json": {Version: 5},
		},
	}, nil)
	uptaneClient.On("TUFVersionState").Return(uptane.TUFVersions{
		ConfigRoot:      1,
		ConfigSnapshot:  2,
		DirectorRoot:    4,
		DirectorTargets: 5,
	}, nil)
	uptaneClient.On("DirectorRoot", uint64(3)).Return(root3, nil)
	uptaneClient.On("DirectorRoot", uint64(4)).Return(root4, nil)
	uptaneClient.On("TargetFile", "datadog/2/APM_SAMPLING/id/1").Return(fileAPM1, nil)
	uptaneClient.On("TargetFile", "datadog/2/APM_SAMPLING/id/2").Return(fileAPM2, nil)
	uptaneClient.On("Update", lastConfigResponse).Return(nil)
	api.On("Fetch", mock.Anything, &pbgo.LatestConfigsRequest{
		Hostname:                     service.hostname,
		AgentVersion:                 version.AgentVersion,
		CurrentConfigRootVersion:     1,
		CurrentConfigSnapshotVersion: 2,
		CurrentDirectorRootVersion:   4,
		Products:                     []string{},
		NewProducts: []string{
			string(rdata.ProductAPMSampling),
		},
		ActiveClients:      []*pbgo.Client{client},
		BackendClientState: []byte(`test_state`),
		HasError:           false,
		Error:              "",
		OrgUuid:            "abcdef",
	}).Return(lastConfigResponse, nil)

	service.clients.seen(client) // Avoid blocking on channel sending when nothing is at the other end
	configResponse, err := service.ClientGetConfigs(context.Background(), &pbgo.ClientGetConfigsRequest{Client: client})
	assert.NoError(t, err)
	assert.ElementsMatch(t, [][]byte{canonicalRoot3, canonicalRoot4}, configResponse.Roots)
	assert.ElementsMatch(t, []*pbgo.File{{Path: "datadog/2/APM_SAMPLING/id/1", Raw: fileAPM1}, {Path: "datadog/2/APM_SAMPLING/id/2", Raw: fileAPM2}}, configResponse.TargetFiles)
	assert.Equal(t, canonicalTargets, configResponse.Targets)
	assert.ElementsMatch(t,
		configResponse.ClientConfigs,
		[]string{
			"datadog/2/APM_SAMPLING/id/1",
			"datadog/2/APM_SAMPLING/id/2",
		},
	)
	err = service.refresh()
	assert.NoError(t, err)

	stateResponse, err := service.ConfigGetState()
	assert.NoError(t, err)
	assert.Equal(t, 5, len(stateResponse.ConfigState))
	assert.Equal(t, uint64(5), stateResponse.ConfigState["role1.json"].Version)
	assert.Equal(t, 2, len(stateResponse.DirectorState))
	api.AssertExpectations(t)
	uptaneClient.AssertExpectations(t)
}

// Test for client predicates
func TestServiceClientPredicates(t *testing.T) {
	clientID := "client-id"
	runtimeID := "runtime-id"
	runtimeIDFail := runtimeID + "_fail"

	assert := assert.New(t)
	clock := clock.NewMock()
	lastConfigResponse := &pbgo.LatestConfigsResponse{
		TargetFiles: []*pbgo.File{{Path: "test"}},
	}
	uptaneClient := &mockUptane{}
	api := &mockAPI{}

	service := newTestService(t, api, uptaneClient, clock)

	client := &pbgo.Client{
		Id: clientID,
		State: &pbgo.ClientState{
			RootVersion: 2,
		},
		Products: []string{
			string(rdata.ProductAPMSampling),
		},
		IsTracer: true,
		ClientTracer: &pbgo.ClientTracer{
			RuntimeId:  runtimeID,
			Language:   "php",
			Env:        "staging",
			AppVersion: "1",
		},
	}
	uptaneClient.On("StoredOrgUUID").Return("abcdef", nil)
	uptaneClient.On("TargetsMeta").Return([]byte(`{"signed": "testtargets"}`), nil)
	uptaneClient.On("TargetsCustom").Return([]byte(`{"opaque_backend_state":"dGVzdF9zdGF0ZQ=="}`), nil)

	wrongServiceName := "wrong-service"
	uptaneClient.On("Targets").Return(data.TargetFiles{
		// must be delivered
		"datadog/2/APM_SAMPLING/id/1": {FileMeta: data.FileMeta{Custom: customMeta([]*pbgo.TracerPredicateV1{}, 0)}},
		"datadog/2/APM_SAMPLING/id/2": {FileMeta: data.FileMeta{Custom: customMeta([]*pbgo.TracerPredicateV1{
			{
				RuntimeID: runtimeID,
			},
		}, 0)}},
		// must not be delivered
		"datadog/2/TESTING1/id/1": {FileMeta: data.FileMeta{Custom: customMeta([]*pbgo.TracerPredicateV1{
			{
				RuntimeID: runtimeIDFail,
			},
		}, 0)}},
		"datadog/2/APPSEC/id/1": {FileMeta: data.FileMeta{Custom: customMeta([]*pbgo.TracerPredicateV1{
			{
				Service: wrongServiceName,
			},
		}, 0)}},
	},
		nil,
	)
	uptaneClient.On("TUFVersionState").Return(uptane.TUFVersions{
		DirectorRoot:    1,
		DirectorTargets: 5,
	}, nil)
	uptaneClient.On("TargetFile", "datadog/2/APM_SAMPLING/id/1").Return([]byte(``), nil)
	uptaneClient.On("TargetFile", "datadog/2/APM_SAMPLING/id/2").Return([]byte(``), nil)
	uptaneClient.On("Update", lastConfigResponse).Return(nil)
	api.On("Fetch", mock.Anything, &pbgo.LatestConfigsRequest{
		Hostname:                     service.hostname,
		AgentVersion:                 version.AgentVersion,
		CurrentConfigRootVersion:     0,
		CurrentConfigSnapshotVersion: 0,
		CurrentDirectorRootVersion:   0,
		Products:                     []string{},
		NewProducts: []string{
			string(rdata.ProductAPMSampling),
		},
		ActiveClients:      []*pbgo.Client{client},
		BackendClientState: []byte(`test_state`),
		HasError:           false,
		Error:              "",
		OrgUuid:            "abcdef",
	}).Return(lastConfigResponse, nil)

	service.clients.seen(client) // Avoid blocking on channel sending when nothing is at the other end
	configResponse, err := service.ClientGetConfigs(context.Background(), &pbgo.ClientGetConfigsRequest{Client: client})
	assert.NoError(err)
	assert.ElementsMatch(
		configResponse.ClientConfigs,
		[]string{
			"datadog/2/APM_SAMPLING/id/1",
			"datadog/2/APM_SAMPLING/id/2",
		},
	)
	err = service.refresh()
	assert.NoError(err)

	api.AssertExpectations(t)
	uptaneClient.AssertExpectations(t)
}

func TestServiceGetRefreshIntervalNone(t *testing.T) {
	api := &mockAPI{}
	uptaneClient := &mockUptane{}
	clock := clock.NewMock()
	service := newTestService(t, api, uptaneClient, clock)

	// For this test we'll just send an empty update to save us some work mocking everything.
	// What matters is the data reported by the uptane module for the top targets custom
	// value.
	lastConfigResponse := &pbgo.LatestConfigsResponse{}
	api.On("Fetch", mock.Anything, &pbgo.LatestConfigsRequest{
		Hostname:                     service.hostname,
		AgentVersion:                 version.AgentVersion,
		CurrentConfigSnapshotVersion: 0,
		CurrentConfigRootVersion:     0,
		CurrentDirectorRootVersion:   0,
		Products:                     []string{},
		NewProducts:                  []string{},
		BackendClientState:           []byte("test_state"),
		OrgUuid:                      "abcdef",
	}).Return(lastConfigResponse, nil)

	// No explicit refresh interval is provided by the backend
	testTargetsCustomNoOverride := []byte(`{"opaque_backend_state":"dGVzdF9zdGF0ZQ=="}`)
	uptaneClient.On("StoredOrgUUID").Return("abcdef", nil)
	uptaneClient.On("TUFVersionState").Return(uptane.TUFVersions{}, nil)
	uptaneClient.On("Update", lastConfigResponse).Return(nil)
	uptaneClient.On("TargetsCustom").Return(testTargetsCustomNoOverride, nil)

	err := service.refresh()
	assert.NoError(t, err)
	api.AssertExpectations(t)
	uptaneClient.AssertExpectations(t)
	assert.Equal(t, service.defaultRefreshInterval, time.Minute*1)
	assert.True(t, service.refreshIntervalOverrideAllowed)
}

func TestServiceGetRefreshIntervalValid(t *testing.T) {
	api := &mockAPI{}
	uptaneClient := &mockUptane{}
	clock := clock.NewMock()
	service := newTestService(t, api, uptaneClient, clock)

	// For this test we'll just send an empty update to save us some work mocking everything.
	// What matters is the data reported by the uptane module for the top targets custom
	// value.
	lastConfigResponse := &pbgo.LatestConfigsResponse{}
	api.On("Fetch", mock.Anything, &pbgo.LatestConfigsRequest{
		Hostname:                     service.hostname,
		AgentVersion:                 version.AgentVersion,
		CurrentConfigSnapshotVersion: 0,
		CurrentConfigRootVersion:     0,
		CurrentDirectorRootVersion:   0,
		Products:                     []string{},
		NewProducts:                  []string{},
		BackendClientState:           []byte("test_state"),
		OrgUuid:                      "abcdef",
	}).Return(lastConfigResponse, nil)

	// An acceptable refresh interval is provided by the backend
	testTargetsCustomOk := []byte(`{"opaque_backend_state":"dGVzdF9zdGF0ZQ==", "agent_refresh_interval": 42}`)
	uptaneClient.On("StoredOrgUUID").Return("abcdef", nil)
	uptaneClient.On("TUFVersionState").Return(uptane.TUFVersions{}, nil)
	uptaneClient.On("Update", lastConfigResponse).Return(nil)
	uptaneClient.On("TargetsCustom").Return(testTargetsCustomOk, nil)

	err := service.refresh()
	assert.NoError(t, err)
	api.AssertExpectations(t)
	uptaneClient.AssertExpectations(t)
	assert.Equal(t, service.defaultRefreshInterval, time.Second*42)
	assert.True(t, service.refreshIntervalOverrideAllowed)
}

func TestServiceGetRefreshIntervalTooSmall(t *testing.T) {
	api := &mockAPI{}
	uptaneClient := &mockUptane{}
	clock := clock.NewMock()
	service := newTestService(t, api, uptaneClient, clock)

	// For this test we'll just send an empty update to save us some work mocking everything.
	// What matters is the data reported by the uptane module for the top targets custom
	// value.
	lastConfigResponse := &pbgo.LatestConfigsResponse{}
	api.On("Fetch", mock.Anything, &pbgo.LatestConfigsRequest{
		Hostname:                     service.hostname,
		AgentVersion:                 version.AgentVersion,
		CurrentConfigSnapshotVersion: 0,
		CurrentConfigRootVersion:     0,
		CurrentDirectorRootVersion:   0,
		Products:                     []string{},
		NewProducts:                  []string{},
		BackendClientState:           []byte("test_state"),
		OrgUuid:                      "abcdef",
	}).Return(lastConfigResponse, nil)

	// A too small refresh interval is provided by the backend (the refresh interval should not change)
	testTargetsCustomOverrideOutOfRangeSmall := []byte(`{"opaque_backend_state":"dGVzdF9zdGF0ZQ==", "agent_refresh_interval": -1}`)
	uptaneClient.On("StoredOrgUUID").Return("abcdef", nil)
	uptaneClient.On("TUFVersionState").Return(uptane.TUFVersions{}, nil)
	uptaneClient.On("Update", lastConfigResponse).Return(nil)
	uptaneClient.On("TargetsCustom").Return(testTargetsCustomOverrideOutOfRangeSmall, nil)

	err := service.refresh()
	assert.NoError(t, err)
	api.AssertExpectations(t)
	uptaneClient.AssertExpectations(t)
	assert.Equal(t, service.defaultRefreshInterval, time.Minute*1)
	assert.True(t, service.refreshIntervalOverrideAllowed)
}

func TestServiceGetRefreshIntervalTooBig(t *testing.T) {
	api := &mockAPI{}
	uptaneClient := &mockUptane{}
	clock := clock.NewMock()
	service := newTestService(t, api, uptaneClient, clock)

	// For this test we'll just send an empty update to save us some work mocking everything.
	// What matters is the data reported by the uptane module for the top targets custom
	// value.
	lastConfigResponse := &pbgo.LatestConfigsResponse{}
	api.On("Fetch", mock.Anything, &pbgo.LatestConfigsRequest{
		Hostname:                     service.hostname,
		AgentVersion:                 version.AgentVersion,
		CurrentConfigSnapshotVersion: 0,
		CurrentConfigRootVersion:     0,
		CurrentDirectorRootVersion:   0,
		Products:                     []string{},
		NewProducts:                  []string{},
		BackendClientState:           []byte("test_state"),
		OrgUuid:                      "abcdef",
	}).Return(lastConfigResponse, nil)

	// A too large refresh interval is provided by the backend (the refresh interval should not change)
	testTargetsCustomOverrideOutOfRangeBig := []byte(`{"opaque_backend_state":"dGVzdF9zdGF0ZQ==", "agent_refresh_interval": 500}`)
	uptaneClient.On("StoredOrgUUID").Return("abcdef", nil)
	uptaneClient.On("TUFVersionState").Return(uptane.TUFVersions{}, nil)
	uptaneClient.On("Update", lastConfigResponse).Return(nil)
	uptaneClient.On("TargetsCustom").Return(testTargetsCustomOverrideOutOfRangeBig, nil)

	err := service.refresh()
	assert.NoError(t, err)
	api.AssertExpectations(t)
	uptaneClient.AssertExpectations(t)
	assert.Equal(t, service.defaultRefreshInterval, time.Minute*1)
	assert.True(t, service.refreshIntervalOverrideAllowed)
}

func TestServiceGetRefreshIntervalNoOverrideAllowed(t *testing.T) {
	api := &mockAPI{}
	uptaneClient := &mockUptane{}
	clock := clock.NewMock()
	service := newTestService(t, api, uptaneClient, clock)

	// Mock that customers set the value, making overrides not allowed
	service.refreshIntervalOverrideAllowed = false

	// For this test we'll just send an empty update to save us some work mocking everything.
	// What matters is the data reported by the uptane module for the top targets custom
	// value and how that interacts with the fact we mocked a customer provided override
	lastConfigResponse := &pbgo.LatestConfigsResponse{}
	api.On("Fetch", mock.Anything, &pbgo.LatestConfigsRequest{
		Hostname:                     service.hostname,
		AgentVersion:                 version.AgentVersion,
		CurrentConfigSnapshotVersion: 0,
		CurrentConfigRootVersion:     0,
		CurrentDirectorRootVersion:   0,
		Products:                     []string{},
		NewProducts:                  []string{},
		BackendClientState:           []byte("test_state"),
		OrgUuid:                      "abcdef",
	}).Return(lastConfigResponse, nil)

	// An interval is provided, but it should not be applied
	testTargetsCustomOk := []byte(`{"opaque_backend_state":"dGVzdF9zdGF0ZQ==", "agent_refresh_interval": 42}`)
	uptaneClient.On("StoredOrgUUID").Return("abcdef", nil)
	uptaneClient.On("TUFVersionState").Return(uptane.TUFVersions{}, nil)
	uptaneClient.On("Update", lastConfigResponse).Return(nil)
	uptaneClient.On("TargetsCustom").Return(testTargetsCustomOk, nil)

	err := service.refresh()
	assert.NoError(t, err)
	api.AssertExpectations(t)
	uptaneClient.AssertExpectations(t)
	assert.Equal(t, service.defaultRefreshInterval, time.Minute*1)
	assert.False(t, service.refreshIntervalOverrideAllowed)
}

// TestConfigExpiration tests that the agent properly filters expired configuration ID's
// when processing a request from a downstream client.
func TestConfigExpiration(t *testing.T) {
	clientID := "client-id"
	runtimeID := "runtime-id"

	assert := assert.New(t)
	clock := clock.NewMock()
	lastConfigResponse := &pbgo.LatestConfigsResponse{
		TargetFiles: []*pbgo.File{{Path: "test"}},
	}
	uptaneClient := &mockUptane{}
	api := &mockAPI{}

	service := newTestService(t, api, uptaneClient, clock)

	client := &pbgo.Client{
		Id: clientID,
		State: &pbgo.ClientState{
			RootVersion: 2,
		},
		Products: []string{
			string(rdata.ProductAPMSampling),
		},
		IsTracer: true,
		ClientTracer: &pbgo.ClientTracer{
			RuntimeId:  runtimeID,
			Language:   "php",
			Env:        "staging",
			AppVersion: "1",
		},
	}
	uptaneClient.On("StoredOrgUUID").Return("abcdef", nil)
	uptaneClient.On("TargetsMeta").Return([]byte(`{"signed": "testtargets"}`), nil)
	uptaneClient.On("TargetsCustom").Return([]byte(`{"opaque_backend_state":"dGVzdF9zdGF0ZQ=="}`), nil)
	uptaneClient.On("Targets").Return(data.TargetFiles{
		// must be delivered
		"datadog/2/APM_SAMPLING/id/1": {FileMeta: data.FileMeta{Custom: customMeta(nil, 0)}},
		// must not be delivered - expiration date is 9/21/2022
		"datadog/2/APM_SAMPLING/id/2": {FileMeta: data.FileMeta{Custom: customMeta(nil, 1663732800)}},
	},
		nil,
	)
	uptaneClient.On("TUFVersionState").Return(uptane.TUFVersions{
		DirectorRoot:    1,
		DirectorTargets: 5,
	}, nil)
	uptaneClient.On("TargetFile", "datadog/2/APM_SAMPLING/id/1").Return([]byte(``), nil)
	uptaneClient.On("TargetFile", "datadog/2/APM_SAMPLING/id/2").Return([]byte(``), nil)
	uptaneClient.On("Update", lastConfigResponse).Return(nil)
	api.On("Fetch", mock.Anything, &pbgo.LatestConfigsRequest{
		Hostname:                     service.hostname,
		AgentVersion:                 version.AgentVersion,
		CurrentConfigRootVersion:     0,
		CurrentConfigSnapshotVersion: 0,
		CurrentDirectorRootVersion:   0,
		Products:                     []string{},
		NewProducts: []string{
			string(rdata.ProductAPMSampling),
		},
		ActiveClients:      []*pbgo.Client{client},
		BackendClientState: []byte(`test_state`),
		HasError:           false,
		Error:              "",
		OrgUuid:            "abcdef",
	}).Return(lastConfigResponse, nil)

	service.clients.seen(client) // Avoid blocking on channel sending when nothing is at the other end
	configResponse, err := service.ClientGetConfigs(context.Background(), &pbgo.ClientGetConfigsRequest{Client: client})
	assert.NoError(err)
	assert.ElementsMatch(
		configResponse.ClientConfigs,
		[]string{
			"datadog/2/APM_SAMPLING/id/1",
		},
	)
	err = service.refresh()
	assert.NoError(err)

	api.AssertExpectations(t)
	uptaneClient.AssertExpectations(t)
}
