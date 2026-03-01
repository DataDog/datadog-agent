// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package service

import (
	"context"
	_ "embed"
	"encoding/base32"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"regexp"
	"runtime/pprof"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	httpapi "github.com/DataDog/datadog-agent/pkg/config/remote/api"

	"github.com/DataDog/go-tuf/data"
	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	rdata "github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/config/remote/uptane"
	"github.com/DataDog/datadog-agent/pkg/proto/msgpgo"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/uuid"
)

type mockAPI struct {
	mock.Mock
}

const (
	httpError    = "api: simulated HTTP error"
	agentVersion = "9.9.9"
	testEnv      = "test-env"
)

// Setup overrides for tests
func init() {
	uuid.GetUUID = func() string {
		// Avoid using the runner's UUID
		return "test-uuid"
	}
}

func (m *mockAPI) Fetch(ctx context.Context, request *pbgo.LatestConfigsRequest) (*pbgo.LatestConfigsResponse, error) {
	args := m.Called(ctx, request)
	return args.Get(0).(*pbgo.LatestConfigsResponse), args.Error(1)
}

func (m *mockAPI) FetchOrgData(ctx context.Context) (*pbgo.OrgDataResponse, error) {
	args := m.Called(ctx)
	return args.Get(0).(*pbgo.OrgDataResponse), args.Error(1)
}

func (m *mockAPI) FetchOrgStatus(ctx context.Context) (*pbgo.OrgStatusResponse, error) {
	args := m.Called(ctx)
	return args.Get(0).(*pbgo.OrgStatusResponse), args.Error(1)
}

func (m *mockAPI) UpdatePARJWT(jwt string) {
	m.Called(jwt)
}

func (m *mockAPI) UpdateAPIKey(apiKey string) {
	m.Called(apiKey)
}

type mockUptane struct {
	mock.Mock
}

type mockCoreAgentUptane struct {
	mockUptane
}

func (m *mockCoreAgentUptane) Update(response *pbgo.LatestConfigsResponse) error {
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

func (m *mockUptane) TargetFiles(files []string) (map[string][]byte, error) {
	args := m.Called(files)
	return args.Get(0).(map[string][]byte), args.Error(1)
}

func (m *mockUptane) TargetsMeta() ([]byte, error) {
	args := m.Called()
	return args.Get(0).([]byte), args.Error(1)
}

func (m *mockUptane) UnsafeTargetsMeta() ([]byte, error) {
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

func (m *mockUptane) TimestampExpires() (time.Time, error) {
	args := m.Called()
	return args.Get(0).(time.Time), args.Error(1)
}

func (m *mockUptane) Close() error {
	return nil
}

func (m *mockUptane) GetTransactionalStoreMetadata() (*uptane.Metadata, error) {
	args := m.Called()
	return args.Get(0).(*uptane.Metadata), args.Error(1)
}

type telemetryReporter struct {
	timeouts   atomic.Int64
	rateLimits atomic.Int64

	subscriptionsActiveGauge        atomic.Int64
	subscriptionClientsTrackedGauge atomic.Int64
	subscriptionsConnected          atomic.Int64
	subscriptionsDisconnected       atomic.Int64
}

func (t *telemetryReporter) IncConfigSubscriptionsConnectedCounter() {
	t.subscriptionsConnected.Add(1)
}
func (t *telemetryReporter) IncConfigSubscriptionsDisconnectedCounter() {
	t.subscriptionsDisconnected.Add(1)
}
func (t *telemetryReporter) IncRateLimit() {
	t.rateLimits.Add(1)
}
func (t *telemetryReporter) IncTimeout() {
	t.timeouts.Add(1)
}
func (t *telemetryReporter) SetConfigSubscriptionClientsTracked(value int) {
	t.subscriptionClientsTrackedGauge.Store(int64(value))
}
func (t *telemetryReporter) SetConfigSubscriptionsActive(value int) {
	t.subscriptionsActiveGauge.Store(int64(value))
}

var _ RcTelemetryReporter = (*telemetryReporter)(nil)

var testRCKey = msgpgo.RemoteConfigKey{
	AppKey:     "fake_key",
	OrgID:      2,
	Datacenter: "dd.com",
}

func uptaneFactoryOption(coreAgentUptane *mockCoreAgentUptane) Option {
	return withUptaneFactory(func(_ *uptane.Metadata) (coreAgentUptaneClient, error) {
		return coreAgentUptane, nil // no DB opened in tests
	})
}

func newTestService(t *testing.T, api *mockAPI, coreAgentUptane *mockCoreAgentUptane, clock clock.Clock, opts ...Option) *CoreAgentService {
	cfg := configmock.New(t)
	cfg.SetWithoutSource("hostname", "test-hostname")

	dir := t.TempDir()
	cfg.SetWithoutSource("run_path", dir)
	serializedKey, _ := testRCKey.MarshalMsg(nil)
	cfg.SetWithoutSource("remote_configuration.key", base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(serializedKey))
	baseRawURL := "https://localhost"
	traceAgentEnv := testEnv
	mockTelemetryReporter := &telemetryReporter{}

	options := []Option{
		uptaneFactoryOption(coreAgentUptane),
		WithDatabaseFileName("test.db"),
		WithTraceAgentEnv(traceAgentEnv),
		WithAPIKey("abc"),
	}
	options = append(options, opts...)
	service, err := NewService(cfg, "Remote Config", baseRawURL, "localhost", getHostTags, mockTelemetryReporter, agentVersion, options...)
	require.NoError(t, err)
	t.Cleanup(func() { service.Stop() })
	service.api = api
	service.clock = clock
	service.mu.uptane = coreAgentUptane
	return service
}

func TestFetchConfigs503And504IncrementsErrCountAndResets(t *testing.T) {
	api := &mockAPI{}
	uptaneClient := &mockCoreAgentUptane{}
	clock := clock.NewMock()
	service := newTestService(t, api, uptaneClient, clock)

	lastConfigResponse := &pbgo.LatestConfigsResponse{
		TargetFiles: []*pbgo.File{{Path: "test"}},
	}

	api.On("Fetch", mock.Anything, &pbgo.LatestConfigsRequest{
		Hostname:                     service.hostname,
		TraceAgentEnv:                testEnv,
		AgentUuid:                    "test-uuid",
		AgentVersion:                 agentVersion,
		CurrentConfigSnapshotVersion: 0,
		CurrentConfigRootVersion:     0,
		CurrentDirectorRootVersion:   0,
		Products:                     []string{},
		NewProducts:                  []string{},
		OrgUuid:                      "abcdef",
		Tags:                         getHostTags(),
	}).Return(lastConfigResponse, httpapi.ErrGatewayTimeout)
	uptaneClient.On("StoredOrgUUID").Return("abcdef", nil)
	uptaneClient.On("TUFVersionState").Return(uptane.TUFVersions{}, nil)
	uptaneClient.On("Update", lastConfigResponse).Return(nil)
	uptaneClient.On("TargetsCustom").Return([]byte{}, nil)

	// There should be no errors at the start
	assert.Equal(t, service.mu.fetchConfigs503And504ErrCount, uint64(0))
	err := service.refresh()
	assert.NotNil(t, err)
	assert.Equal(t, service.mu.fetchConfigs503And504ErrCount, uint64(1))

	api.On("Fetch", mock.Anything, &pbgo.LatestConfigsRequest{
		Hostname:                     service.hostname,
		TraceAgentEnv:                testEnv,
		AgentUuid:                    "test-uuid",
		AgentVersion:                 agentVersion,
		CurrentConfigSnapshotVersion: 0,
		CurrentConfigRootVersion:     0,
		CurrentDirectorRootVersion:   0,
		Products:                     []string{},
		NewProducts:                  []string{},
		HasError:                     true,
		Error:                        fmt.Sprintf("api: %v", httpapi.ErrGatewayTimeout.Error()),
		OrgUuid:                      "abcdef",
		Tags:                         getHostTags(),
	}).Return(lastConfigResponse, httpapi.ErrServiceUnavailable)
	uptaneClient.On("StoredOrgUUID").Return("abcdef", nil)
	uptaneClient.On("TUFVersionState").Return(uptane.TUFVersions{}, nil)
	uptaneClient.On("Update", lastConfigResponse).Return(nil)
	uptaneClient.On("TargetsCustom").Return([]byte{}, nil)

	err = service.refresh()
	assert.NotNil(t, err)
	assert.Equal(t, service.mu.fetchConfigs503And504ErrCount, uint64(2))

	// After a successful refresh, the error count should be reset
	api.On("Fetch", mock.Anything, &pbgo.LatestConfigsRequest{
		Hostname:                     service.hostname,
		TraceAgentEnv:                testEnv,
		AgentUuid:                    "test-uuid",
		AgentVersion:                 agentVersion,
		CurrentConfigSnapshotVersion: 0,
		CurrentConfigRootVersion:     0,
		CurrentDirectorRootVersion:   0,
		Products:                     []string{},
		NewProducts:                  []string{},
		HasError:                     true,
		Error:                        fmt.Sprintf("api: %v", httpapi.ErrServiceUnavailable.Error()),
		OrgUuid:                      "abcdef",
		Tags:                         getHostTags(),
	}).Return(lastConfigResponse, nil)
	uptaneClient.On("StoredOrgUUID").Return("abcdef", nil)
	uptaneClient.On("TUFVersionState").Return(uptane.TUFVersions{}, nil)
	uptaneClient.On("Update", lastConfigResponse).Return(nil)
	uptaneClient.On("TargetsCustom").Return([]byte{}, nil)

	err = service.refresh()
	assert.Nil(t, err)
	assert.Equal(t, service.mu.fetchConfigs503And504ErrCount, uint64(0))
}

func TestFetchOrgStatus503And504IncrementsErrCount(t *testing.T) {
	api := &mockAPI{}
	clock := clock.NewMock()
	uptaneClient := &mockCoreAgentUptane{}
	service := newTestService(t, api, uptaneClient, clock)

	response := &pbgo.OrgStatusResponse{
		Enabled:    true,
		Authorized: true,
	}
	// start with no previous errors
	assert.Equal(t, service.orgStatusPoller.mu.fetchOrgStatus503And504ErrCount, uint64(0))

	api.On("FetchOrgStatus", mock.Anything).Return(response, httpapi.ErrGatewayTimeout)
	service.orgStatusPoller.poll(service.api, service.rcType)
	assert.Equal(t, service.orgStatusPoller.mu.fetchOrgStatus503And504ErrCount, uint64(1))

	assert.Nil(t, service.orgStatusPoller.getPreviousStatus())
	api.On("FetchOrgStatus", mock.Anything).Return(response, httpapi.ErrGatewayTimeout)

	service.orgStatusPoller.poll(service.api, service.rcType)
	assert.Equal(t, service.orgStatusPoller.mu.fetchOrgStatus503And504ErrCount, uint64(2))
}

func TestFetchOrgStatusSuccessResetsErrorCount(t *testing.T) {
	api := &mockAPI{}
	clock := clock.NewMock()
	uptaneClient := &mockCoreAgentUptane{}
	service := newTestService(t, api, uptaneClient, clock)

	service.orgStatusPoller.mu.fetchOrgStatus503And504ErrCount = 1
	response := &pbgo.OrgStatusResponse{
		Enabled:    true,
		Authorized: true,
	}
	// start with 1 error
	assert.Equal(t, service.orgStatusPoller.mu.fetchOrgStatus503And504ErrCount, uint64(1))

	assert.Nil(t, service.orgStatusPoller.getPreviousStatus())
	api.On("FetchOrgStatus", mock.Anything).Return(response, nil)

	service.orgStatusPoller.poll(service.api, service.rcType)
	assert.Equal(t, service.orgStatusPoller.mu.fetchOrgStatus503And504ErrCount, uint64(0))
}

func TestServiceBackoffFailure(t *testing.T) {
	api := &mockAPI{}
	uptaneClient := &mockCoreAgentUptane{}
	clock := clock.NewMock()
	service := newTestService(t, api, uptaneClient, clock)

	lastConfigResponse := &pbgo.LatestConfigsResponse{
		TargetFiles: []*pbgo.File{{Path: "test"}},
	}

	api.On("Fetch", mock.Anything, &pbgo.LatestConfigsRequest{
		Hostname:                     service.hostname,
		TraceAgentEnv:                testEnv,
		AgentUuid:                    "test-uuid",
		AgentVersion:                 agentVersion,
		CurrentConfigSnapshotVersion: 0,
		CurrentConfigRootVersion:     0,
		CurrentDirectorRootVersion:   0,
		Products:                     []string{},
		NewProducts:                  []string{},
		OrgUuid:                      "abcdef",
		Tags:                         getHostTags(),
	}).Return(lastConfigResponse, errors.New("simulated HTTP error"))
	uptaneClient.On("StoredOrgUUID").Return("abcdef", nil)
	uptaneClient.On("TUFVersionState").Return(uptane.TUFVersions{}, nil)
	uptaneClient.On("Update", lastConfigResponse).Return(nil)
	uptaneClient.On("TargetsCustom").Return([]byte{}, nil)

	// We'll set the default interal to 1 second to make math less hard
	service.mu.defaultRefreshInterval = 1 * time.Second

	// There should be no errors at the start
	assert.Equal(t, service.mu.backoffErrorCount, 0)

	err := service.refresh()
	assert.NotNil(t, err)

	// Sending the http error too
	api.On("Fetch", mock.Anything, &pbgo.LatestConfigsRequest{
		Hostname:                     service.hostname,
		TraceAgentEnv:                testEnv,
		AgentUuid:                    "test-uuid",
		AgentVersion:                 agentVersion,
		CurrentConfigSnapshotVersion: 0,
		CurrentConfigRootVersion:     0,
		CurrentDirectorRootVersion:   0,
		Products:                     []string{},
		NewProducts:                  []string{},
		HasError:                     true,
		Error:                        httpError,
		OrgUuid:                      "abcdef",
		Tags:                         getHostTags(),
	}).Return(lastConfigResponse, errors.New("simulated HTTP error"))
	uptaneClient.On("StoredOrgUUID").Return("abcdef", nil)
	uptaneClient.On("TUFVersionState").Return(uptane.TUFVersions{}, nil)
	uptaneClient.On("Update", lastConfigResponse).Return(nil)
	uptaneClient.On("TargetsCustom").Return([]byte{}, nil)

	// We should be tracking an error now. With the default backoff config, our refresh interval
	// should be somewhere in the range of 1 + [30,60], so [31,61]
	assert.Equal(t, service.mu.backoffErrorCount, 1)
	refreshInterval := service.calculateRefreshInterval()
	assert.GreaterOrEqual(t, refreshInterval, 31*time.Second)
	assert.LessOrEqual(t, refreshInterval, 61*time.Second)

	err = service.refresh()
	assert.NotNil(t, err)

	// Now we're looking at  1 + [60, 120], so [61,121]
	assert.Equal(t, service.mu.backoffErrorCount, 2)
	refreshInterval = service.calculateRefreshInterval()
	assert.GreaterOrEqual(t, refreshInterval, 61*time.Second)
	assert.LessOrEqual(t, refreshInterval, 121*time.Second)

	err = service.refresh()
	assert.NotNil(t, err)

	// After one more we're looking at  1 + [120, 240], so [121,241]
	assert.Equal(t, service.mu.backoffErrorCount, 3)
	refreshInterval = service.calculateRefreshInterval()
	assert.GreaterOrEqual(t, refreshInterval, 121*time.Second)
	assert.LessOrEqual(t, refreshInterval, 241*time.Second)
}

func TestServiceBackoffFailureRecovery(t *testing.T) {
	api := &mockAPI{}
	uptaneClient := &mockCoreAgentUptane{}
	clock := clock.NewMock()
	service := newTestService(t, api, uptaneClient, clock)

	lastConfigResponse := &pbgo.LatestConfigsResponse{
		TargetFiles: []*pbgo.File{{Path: "test"}},
	}

	api = &mockAPI{}
	api.On("Fetch", mock.Anything, &pbgo.LatestConfigsRequest{
		Hostname:                     service.hostname,
		TraceAgentEnv:                testEnv,
		AgentUuid:                    "test-uuid",
		AgentVersion:                 agentVersion,
		CurrentConfigSnapshotVersion: 0,
		CurrentConfigRootVersion:     0,
		CurrentDirectorRootVersion:   0,
		Products:                     []string{},
		NewProducts:                  []string{},
		OrgUuid:                      "abcdef",
		Tags:                         getHostTags(),
	}).Return(lastConfigResponse, nil)
	uptaneClient.On("StoredOrgUUID").Return("abcdef", nil)
	uptaneClient.On("TUFVersionState").Return(uptane.TUFVersions{}, nil)
	uptaneClient.On("Update", lastConfigResponse).Return(nil)
	uptaneClient.On("TargetsCustom").Return([]byte{}, nil)
	service.api = api

	// Artificially set the backoff error count so we can test recovery
	service.mu.backoffErrorCount = 3

	// We'll set the default interal to 1 second to make math less hard
	service.mu.defaultRefreshInterval = 1 * time.Second

	require.NoError(t, service.refresh())

	// Our recovery interval is 2, so we should step back to the [31,61] range
	assert.Equal(t, service.mu.backoffErrorCount, 1)
	refreshInterval := service.calculateRefreshInterval()
	assert.GreaterOrEqual(t, refreshInterval, 31*time.Second)
	assert.LessOrEqual(t, refreshInterval, 61*time.Second)

	errCh := make(chan error)
	go func() { errCh <- service.refresh() }()
	// Wait for the refresh call to be blocked before advancing the time. The
	// clock library doesn't have a nice way to detect if the goroutine is
	// blocked on a sleep, so we look at the goroutine stack trace.
	const sleepPat = `(?s)chan receive([^\n]+\n)+[^\n]+clock\.\(\*Mock\)\.Sleep`
	sleepRegexp := regexp.MustCompile(sleepPat)
	inSleep := func() bool { return sleepRegexp.MatchString(dumpAllStacks()) }
	require.Eventually(t, inSleep, 10*time.Second, time.Millisecond)

	// Advance the clock by 1 second to trigger the next refresh.
	clock.Add(1 * time.Second)
	require.NoError(t, <-errCh)

	// After a 2nd success, we'll be back to not having a backoff added.
	assert.Equal(t, service.mu.backoffErrorCount, 0)
	refreshInterval = service.calculateRefreshInterval()
	assert.Equal(t, 1*time.Second, refreshInterval)
}

func dumpAllStacks() string {
	var buf strings.Builder
	pprof.Lookup("goroutine").WriteTo(&buf, 2)
	return buf.String()
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
	uptaneClient := &mockCoreAgentUptane{}
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

func TestClientGetConfigsProvidesEmptyResponseForExpiredSignature(t *testing.T) {
	api := &mockAPI{}
	uptaneClient := &mockCoreAgentUptane{}
	clock := clock.NewMock()
	service := newTestService(t, api, uptaneClient, clock)

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
	uptaneClient.On("TUFVersionState").Return(uptane.TUFVersions{}, nil)
	uptaneClient.On("TimestampExpires").Return(clock.Now().Add(-1*time.Hour), nil)
	uptaneClient.On("UnsafeTargetsMeta").Return([]byte{}, nil)

	// The key here is that if we've never initialized the TUF repository, we shouldn't be sending any sort of expired message
	// because the expired message check requires the TUF repository have Director Meta data.
	//
	// Checks in this state should look like the version of the client is unchanged (both will report 0 as the director targets meta, because
	// no such file exists).
	service.clients.seen(client)
	response, err := service.ClientGetConfigs(context.Background(), &pbgo.ClientGetConfigsRequest{Client: client})
	assert.NoError(t, err)
	assert.Len(t, response.ClientConfigs, 0)
	assert.Len(t, response.TargetFiles, 0)
	assert.Len(t, response.Roots, 0)
	assert.Len(t, response.Targets, 0)
	assert.Equal(t, pbgo.ConfigStatus_CONFIG_STATUS_OK, response.ConfigStatus)

	// If the repository is initialized, we should now return the expired message
	service.mu.firstUpdate = false
	response, err = service.ClientGetConfigs(context.Background(), &pbgo.ClientGetConfigsRequest{Client: client})
	assert.NoError(t, err)
	assert.Equal(t, pbgo.ConfigStatus_CONFIG_STATUS_EXPIRED, response.ConfigStatus)
}

func TestService(t *testing.T) {
	api := &mockAPI{}
	uptaneClient := &mockCoreAgentUptane{}
	clock := clock.NewMock()
	service := newTestService(t, api, uptaneClient, clock)

	lastConfigResponse := &pbgo.LatestConfigsResponse{
		TargetFiles: []*pbgo.File{{Path: "test"}},
	}
	api.On("Fetch", mock.Anything, &pbgo.LatestConfigsRequest{
		Hostname:                     service.hostname,
		TraceAgentEnv:                testEnv,
		AgentUuid:                    "test-uuid",
		AgentVersion:                 agentVersion,
		CurrentConfigSnapshotVersion: 0,
		CurrentConfigRootVersion:     0,
		CurrentDirectorRootVersion:   0,
		Products:                     []string{},
		NewProducts:                  []string{},
		OrgUuid:                      "abcdef",
		Tags:                         getHostTags(),
	}).Return(lastConfigResponse, nil)
	uptaneClient.On("StoredOrgUUID").Return("abcdef", nil)
	uptaneClient.On("TUFVersionState").Return(uptane.TUFVersions{}, nil)
	uptaneClient.On("Update", lastConfigResponse).Return(nil)
	uptaneClient.On("TargetsCustom").Return([]byte{}, nil)

	err := service.refresh()
	assert.NoError(t, err)

	api.AssertExpectations(t)
	uptaneClient.AssertExpectations(t)

	*uptaneClient = mockCoreAgentUptane{}
	*api = mockAPI{}

	root3 := []byte(`{"signatures": "testroot3", "signed": "signed"}`)
	canonicalRoot3 := []byte(`{"signatures": "testroot3", "signed": "signed"}`)
	root4 := []byte(`{"signed": "signed", "signatures": "testroot4"}`)
	canonicalRoot4 := []byte(`{"signed": "signed", "signatures": "testroot4"}`)
	targets := []byte(`{"signatures": "testtargets", "signed": "stuff"}`)
	canonicalTargets := []byte(`{"signatures": "testtargets", "signed": "stuff"}`)
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
	uptaneClient.On("TimestampExpires").Return(time.Now().Add(1*time.Hour), nil)

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

	uptaneClient.On("TargetFiles", mock.MatchedBy(listsEqual([]string{"datadog/2/APM_SAMPLING/id/1", "datadog/2/APM_SAMPLING/id/2"}))).Return(map[string][]byte{"datadog/2/APM_SAMPLING/id/1": fileAPM1, "datadog/2/APM_SAMPLING/id/2": fileAPM2}, nil)
	uptaneClient.On("Update", lastConfigResponse).Return(nil)
	api.On("Fetch", mock.Anything, &pbgo.LatestConfigsRequest{
		Hostname:                     service.hostname,
		AgentUuid:                    "test-uuid",
		TraceAgentEnv:                testEnv,
		AgentVersion:                 agentVersion,
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
		Tags:               getHostTags(),
	}).Return(lastConfigResponse, nil)

	service.clients.seen(client) // Avoid blocking on channel sending when nothing is at the other end
	configResponse, err := service.ClientGetConfigs(context.Background(), &pbgo.ClientGetConfigsRequest{Client: client})
	assert.NoError(t, err)
	assert.ElementsMatch(t, [][]byte{canonicalRoot3, canonicalRoot4}, configResponse.Roots)
	assert.ElementsMatch(t, []*pbgo.File{{Path: "datadog/2/APM_SAMPLING/id/1", Raw: fileAPM1}, {Path: "datadog/2/APM_SAMPLING/id/2", Raw: fileAPM2}}, configResponse.TargetFiles, nil)
	assert.Equal(t, canonicalTargets, configResponse.Targets)
	assert.ElementsMatch(t,
		configResponse.ClientConfigs,
		[]string{
			"datadog/2/APM_SAMPLING/id/1",
			"datadog/2/APM_SAMPLING/id/2",
		},
	)
	assert.ElementsMatch(t,
		configResponse.ConfigStatus,
		pbgo.ConfigStatus_CONFIG_STATUS_OK,
	)

	// Advance the clock by 1 second to ensure we don't block waiting for
	// the minimum refresh interval.
	clock.Add(1 * time.Second)
	err = service.refresh()
	assert.NoError(t, err)

	stateResponse, err := service.ConfigGetState()
	assert.NoError(t, err)
	assert.Equal(t, 5, len(stateResponse.ConfigState))
	assert.Equal(t, uint64(5), stateResponse.ConfigState["role1.json"].Version)
	assert.Equal(t, 2, len(stateResponse.DirectorState))
	api.AssertExpectations(t)
	uptaneClient.AssertExpectations(t)

	uptaneClient.On("GetTransactionalStoreMetadata").Return(&uptane.Metadata{
		Path:         path.Join(t.TempDir(), "test.db"),
		AgentVersion: agentVersion,
		APIKey:       "abc",
		URL:          "https://localhost",
	}, nil)

	_, err = service.ConfigResetState()
	assert.NoError(t, err)
	uptaneClient.AssertExpectations(t)

	// The state should be reset, so we should not be able to get the state again
	// because the state is empty.
	_, err = service.ConfigGetState()
	assert.Error(t, err)
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
	uptaneClient := &mockCoreAgentUptane{}
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
		"datadog/2/APM_SAMPLING/id/1": {Custom: customMeta([]*pbgo.TracerPredicateV1{}, 0)},
		"datadog/2/APM_SAMPLING/id/2": {Custom: customMeta([]*pbgo.TracerPredicateV1{
			{
				RuntimeID: runtimeID,
			},
		}, 0)},
		// must not be delivered
		"datadog/2/TESTING1/id/1": {Custom: customMeta([]*pbgo.TracerPredicateV1{
			{
				RuntimeID: runtimeIDFail,
			},
		}, 0)},
		"datadog/2/APPSEC/id/1": {Custom: customMeta([]*pbgo.TracerPredicateV1{
			{
				Service: wrongServiceName,
			},
		}, 0)},
	},
		nil,
	)
	uptaneClient.On("TUFVersionState").Return(uptane.TUFVersions{
		DirectorRoot:    1,
		DirectorTargets: 5,
	}, nil)
	uptaneClient.On("TargetFiles", mock.MatchedBy(listsEqual([]string{"datadog/2/APM_SAMPLING/id/1", "datadog/2/APM_SAMPLING/id/2"}))).Return(map[string][]byte{"datadog/2/APM_SAMPLING/id/1": []byte(``), "datadog/2/APM_SAMPLING/id/2": []byte(``)}, nil)
	uptaneClient.On("Update", lastConfigResponse).Return(nil)
	api.On("Fetch", mock.Anything, &pbgo.LatestConfigsRequest{
		Hostname:                     service.hostname,
		TraceAgentEnv:                testEnv,
		AgentUuid:                    "test-uuid",
		AgentVersion:                 agentVersion,
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
		Tags:               getHostTags(),
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
	uptaneClient := &mockCoreAgentUptane{}
	clock := clock.NewMock()
	service := newTestService(t, api, uptaneClient, clock)

	// For this test we'll just send an empty update to save us some work mocking everything.
	// What matters is the data reported by the uptane module for the top targets custom
	// value.
	lastConfigResponse := &pbgo.LatestConfigsResponse{}
	api.On("Fetch", mock.Anything, &pbgo.LatestConfigsRequest{
		Hostname:                     service.hostname,
		TraceAgentEnv:                testEnv,
		AgentUuid:                    "test-uuid",
		AgentVersion:                 agentVersion,
		CurrentConfigSnapshotVersion: 0,
		CurrentConfigRootVersion:     0,
		CurrentDirectorRootVersion:   0,
		Products:                     []string{},
		NewProducts:                  []string{},
		BackendClientState:           []byte("test_state"),
		OrgUuid:                      "abcdef",
		Tags:                         getHostTags(),
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
	assert.Equal(t, service.mu.defaultRefreshInterval, time.Minute*1)
	assert.True(t, service.mu.refreshIntervalOverrideAllowed)
}

func TestServiceGetRefreshIntervalValid(t *testing.T) {
	api := &mockAPI{}
	uptaneClient := &mockCoreAgentUptane{}
	clock := clock.NewMock()
	service := newTestService(t, api, uptaneClient, clock)

	// For this test we'll just send an empty update to save us some work mocking everything.
	// What matters is the data reported by the uptane module for the top targets custom
	// value.
	lastConfigResponse := &pbgo.LatestConfigsResponse{}
	api.On("Fetch", mock.Anything, &pbgo.LatestConfigsRequest{
		Hostname:                     service.hostname,
		TraceAgentEnv:                testEnv,
		AgentUuid:                    "test-uuid",
		AgentVersion:                 agentVersion,
		CurrentConfigSnapshotVersion: 0,
		CurrentConfigRootVersion:     0,
		CurrentDirectorRootVersion:   0,
		Products:                     []string{},
		NewProducts:                  []string{},
		BackendClientState:           []byte("test_state"),
		OrgUuid:                      "abcdef",
		Tags:                         getHostTags(),
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
	assert.Equal(t, service.mu.defaultRefreshInterval, time.Second*42)
	assert.True(t, service.mu.refreshIntervalOverrideAllowed)
}

func TestWithApiKeyUpdate(t *testing.T) {
	api := &mockAPI{}
	uptaneClient := &mockCoreAgentUptane{}
	updatedKey := "notUpdated"

	api.On("UpdateAPIKey", mock.Anything).Run(func(args mock.Arguments) {
		updatedKey = args.Get(0).(string)
	})
	orgResponse := pbgo.OrgDataResponse{
		Uuid: "firstUuid",
	}
	api.On("FetchOrgData", mock.Anything).Return(&orgResponse, nil)
	uptaneClient.On("StoredOrgUUID").Return("firstUuid", nil)

	cfg := configmock.New(t)
	dir := t.TempDir()
	cfg.SetWithoutSource("run_path", dir)

	baseRawURL := "https://localhost"
	mockTelemetryReporter := (&telemetryReporter{})
	options := []Option{
		WithAPIKey("initialKey"),
		uptaneFactoryOption(uptaneClient),
	}
	service, err := NewService(cfg, "Remote Config", baseRawURL, "localhost", getHostTags, mockTelemetryReporter, agentVersion, options...)
	assert.NoError(t, err)
	assert.NotNil(t, service)
	t.Cleanup(func() {
		assert.NoError(t, service.Stop())
		assert.NoError(t, service.Stop()) // ensure idempotency
	})
	service.api = api
	service.mu.uptane = uptaneClient

	cfg.SetWithoutSource("api_key", "updated")
	assert.Equal(t, "updated", updatedKey)

	// We still use the new key even if the new org doesn't match the old org.
	orgResponse.Uuid = "badUuid"
	cfg.SetWithoutSource("api_key", "BAD_ORG")
	assert.Equal(t, "BAD_ORG", updatedKey)

}

func TestServiceGetRefreshIntervalTooSmall(t *testing.T) {
	api := &mockAPI{}
	uptaneClient := &mockCoreAgentUptane{}
	clock := clock.NewMock()
	service := newTestService(t, api, uptaneClient, clock)

	// For this test we'll just send an empty update to save us some work mocking everything.
	// What matters is the data reported by the uptane module for the top targets custom
	// value.
	lastConfigResponse := &pbgo.LatestConfigsResponse{}
	api.On("Fetch", mock.Anything, &pbgo.LatestConfigsRequest{
		Hostname:                     service.hostname,
		TraceAgentEnv:                testEnv,
		AgentUuid:                    "test-uuid",
		AgentVersion:                 agentVersion,
		CurrentConfigSnapshotVersion: 0,
		CurrentConfigRootVersion:     0,
		CurrentDirectorRootVersion:   0,
		Products:                     []string{},
		NewProducts:                  []string{},
		BackendClientState:           []byte("test_state"),
		OrgUuid:                      "abcdef",
		Tags:                         getHostTags(),
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
	assert.Equal(t, service.mu.defaultRefreshInterval, time.Minute*1)
	assert.True(t, service.mu.refreshIntervalOverrideAllowed)
}

func TestServiceGetRefreshIntervalTooBig(t *testing.T) {
	api := &mockAPI{}
	uptaneClient := &mockCoreAgentUptane{}
	clock := clock.NewMock()
	service := newTestService(t, api, uptaneClient, clock)

	// For this test we'll just send an empty update to save us some work mocking everything.
	// What matters is the data reported by the uptane module for the top targets custom
	// value.
	lastConfigResponse := &pbgo.LatestConfigsResponse{}
	api.On("Fetch", mock.Anything, &pbgo.LatestConfigsRequest{
		Hostname:                     service.hostname,
		TraceAgentEnv:                testEnv,
		AgentUuid:                    "test-uuid",
		AgentVersion:                 agentVersion,
		CurrentConfigSnapshotVersion: 0,
		CurrentConfigRootVersion:     0,
		CurrentDirectorRootVersion:   0,
		Products:                     []string{},
		NewProducts:                  []string{},
		BackendClientState:           []byte("test_state"),
		OrgUuid:                      "abcdef",
		Tags:                         getHostTags(),
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
	assert.Equal(t, service.mu.defaultRefreshInterval, time.Minute*1)
	assert.True(t, service.mu.refreshIntervalOverrideAllowed)
}

func TestServiceGetRefreshIntervalNoOverrideAllowed(t *testing.T) {
	api := &mockAPI{}
	uptaneClient := &mockCoreAgentUptane{}
	clock := clock.NewMock()
	service := newTestService(t, api, uptaneClient, clock)

	// Mock that customers set the value, making overrides not allowed
	service.mu.refreshIntervalOverrideAllowed = false

	// For this test we'll just send an empty update to save us some work mocking everything.
	// What matters is the data reported by the uptane module for the top targets custom
	// value and how that interacts with the fact we mocked a customer provided override
	lastConfigResponse := &pbgo.LatestConfigsResponse{}
	api.On("Fetch", mock.Anything, &pbgo.LatestConfigsRequest{
		Hostname:                     service.hostname,
		TraceAgentEnv:                testEnv,
		AgentUuid:                    "test-uuid",
		AgentVersion:                 agentVersion,
		CurrentConfigSnapshotVersion: 0,
		CurrentConfigRootVersion:     0,
		CurrentDirectorRootVersion:   0,
		Products:                     []string{},
		NewProducts:                  []string{},
		BackendClientState:           []byte("test_state"),
		OrgUuid:                      "abcdef",
		Tags:                         getHostTags(),
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
	assert.Equal(t, service.mu.defaultRefreshInterval, time.Minute*1)
	assert.False(t, service.mu.refreshIntervalOverrideAllowed)
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
	uptaneClient := &mockCoreAgentUptane{}
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
		"datadog/2/APM_SAMPLING/id/1": {Custom: customMeta(nil, 0)},
		// must not be delivered - expiration date is 9/21/2022
		"datadog/2/APM_SAMPLING/id/2": {Custom: customMeta(nil, 1663732800)},
	},
		nil,
	)
	uptaneClient.On("TUFVersionState").Return(uptane.TUFVersions{
		DirectorRoot:    1,
		DirectorTargets: 5,
	}, nil)
	uptaneClient.On("TargetFiles", []string{"datadog/2/APM_SAMPLING/id/1"}).Return(map[string][]byte{"datadog/2/APM_SAMPLING/id/1": []byte(``), "datadog/2/APM_SAMPLING/id/2": []byte(``)}, nil)
	uptaneClient.On("Update", lastConfigResponse).Return(nil)
	api.On("Fetch", mock.Anything, &pbgo.LatestConfigsRequest{
		Hostname:                     service.hostname,
		TraceAgentEnv:                testEnv,
		AgentUuid:                    "test-uuid",
		AgentVersion:                 agentVersion,
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
		Tags:               getHostTags(),
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

func TestOrgStatus(t *testing.T) {
	api := &mockAPI{}
	clock := clock.NewMock()
	uptaneClient := &mockCoreAgentUptane{}
	service := newTestService(t, api, uptaneClient, clock)

	response := &pbgo.OrgStatusResponse{
		Enabled:    true,
		Authorized: true,
	}

	assert.Nil(t, service.orgStatusPoller.getPreviousStatus())
	api.On("FetchOrgStatus", mock.Anything).Return(response, nil)

	service.orgStatusPoller.poll(service.api, service.rcType)
	prev := service.orgStatusPoller.getPreviousStatus()
	assert.True(t, prev.Enabled)
	assert.True(t, prev.Authorized)

	api.On("FetchOrgStatus", mock.Anything).Return(nil, errors.New("Error"))
	service.orgStatusPoller.poll(service.api, service.rcType)
	prev = service.orgStatusPoller.getPreviousStatus()
	assert.True(t, prev.Enabled)
	assert.True(t, prev.Authorized)

	response.Authorized = false
	api.On("FetchOrgStatus", mock.Anything).Return(response, nil)
	service.orgStatusPoller.poll(service.api, service.rcType)
	prev = service.orgStatusPoller.getPreviousStatus()
	assert.True(t, prev.Enabled)
	assert.False(t, prev.Authorized)
}

func TestWithTraceAgentEnv(t *testing.T) {
	cfg := configmock.New(t)
	dir := t.TempDir()
	cfg.SetWithoutSource("run_path", dir)

	baseRawURL := "https://localhost"
	traceAgentEnv := "dog"
	mockTelemetryReporter := &telemetryReporter{}
	uptaneClient := &mockCoreAgentUptane{}

	options := []Option{
		WithTraceAgentEnv(traceAgentEnv),
		WithAPIKey("abc"),
		uptaneFactoryOption(uptaneClient),
	}
	service, err := NewService(cfg, "Remote Config", baseRawURL, "localhost", getHostTags, mockTelemetryReporter, agentVersion, options...)
	assert.NoError(t, err)
	assert.Equal(t, "dog", service.traceAgentEnv)
	assert.NotNil(t, service)
	assert.NoError(t, service.Stop())
	assert.NoError(t, service.Stop()) // ensure idempotency
}

func TestWithDatabaseFileName(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetWithoutSource("run_path", "/tmp")

	baseRawURL := "https://localhost"
	mockTelemetryReporter := &telemetryReporter{}

	var uptaneClientMetadata *uptane.Metadata
	options := []Option{
		WithDatabaseFileName("test.db"),
		WithAPIKey("abc"),
		withUptaneFactory(func(md *uptane.Metadata) (coreAgentUptaneClient, error) {
			uptaneClientMetadata = md
			return &mockCoreAgentUptane{}, nil
		}),
	}
	service, err := NewService(cfg, "Remote Config", baseRawURL, "localhost", getHostTags, mockTelemetryReporter, agentVersion, options...)
	assert.NoError(t, err)
	assert.NotNil(t, service)
	t.Cleanup(func() { service.Stop() })

	expectedPath := path.Join("/tmp", "test.db")
	assert.NotNil(t, uptaneClientMetadata)
	assert.Equal(t, expectedPath, uptaneClientMetadata.Path)
}

type refreshIntervalTest struct {
	name                                   string
	interval                               time.Duration
	expected                               time.Duration
	expectedRefreshIntervalOverrideAllowed bool
}

func TestWithRefreshInterval(t *testing.T) {
	tests := []refreshIntervalTest{
		{
			name:                                   "valid interval",
			interval:                               42 * time.Second,
			expected:                               42 * time.Second,
			expectedRefreshIntervalOverrideAllowed: false,
		},
		{
			name:                                   "interval too short",
			interval:                               1 * time.Second,
			expected:                               defaultRefreshInterval,
			expectedRefreshIntervalOverrideAllowed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := configmock.New(t)
			cfg.SetWithoutSource("run_path", "/tmp")

			baseRawURL := "https://localhost"
			mockTelemetryReporter := &telemetryReporter{}

			uptaneClient := &mockCoreAgentUptane{}
			options := []Option{
				WithRefreshInterval(tt.interval, "test.refresh_interval"),
				WithAPIKey("abc"),
				uptaneFactoryOption(uptaneClient),
			}
			service, err := NewService(cfg, "Remote Config", baseRawURL, "localhost", getHostTags, mockTelemetryReporter, agentVersion, options...)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, service.mu.defaultRefreshInterval)
			assert.Equal(t, tt.expectedRefreshIntervalOverrideAllowed, service.mu.refreshIntervalOverrideAllowed)
			assert.NotNil(t, service)
			service.Stop()
		})
	}
}

type maxBackoffIntervalTest struct {
	name     string
	interval time.Duration
	expected time.Duration
}

func TestWithMaxBackoffInterval(t *testing.T) {

	tests := []maxBackoffIntervalTest{
		{
			name:     "valid interval",
			interval: 3 * time.Minute,
			expected: 3 * time.Minute,
		},
		{
			name:     "interval too short",
			interval: 1 * time.Second,
			expected: minimalMaxBackoffTime,
		},
		{
			name:     "interval too long",
			interval: 1 * time.Hour,
			expected: maximalMaxBackoffTime,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opt := WithMaxBackoffInterval(tt.interval, "test.max_backoff_interval")
			defaultOptions := &options{}
			opt(defaultOptions)
			assert.Equal(t, tt.expected, defaultOptions.maxBackoff)
		})
	}
}

type clientCacheBypassLimitTest struct {
	name     string
	limit    int
	expected int
}

func TestWithClientCacheBypassLimit(t *testing.T) {
	tests := []clientCacheBypassLimitTest{
		{
			name:     "valid limit",
			limit:    5,
			expected: 5,
		},
		{
			name:     "limit too small",
			limit:    -1,
			expected: defaultCacheBypassLimit,
		},
		{
			name:     "limit too large",
			limit:    100000,
			expected: defaultCacheBypassLimit,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opt := WithClientCacheBypassLimit(tt.limit, "test.client_cache_bypass_limit")
			defaultOptions := &options{}
			opt(defaultOptions)
			assert.Equal(t, tt.expected, defaultOptions.clientCacheBypassLimit)
		})
	}
}

type clientTTLTest struct {
	name     string
	ttl      time.Duration
	expected time.Duration
}

func TestWithClientTTL(t *testing.T) {
	tests := []clientTTLTest{
		{
			name:     "valid ttl",
			ttl:      10 * time.Second,
			expected: 10 * time.Second,
		},
		{
			name:     "ttl too short",
			ttl:      1 * time.Second,
			expected: defaultClientsTTL,
		},
		{
			name:     "ttl too long",
			ttl:      1 * time.Hour,
			expected: defaultClientsTTL,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opt := WithClientTTL(tt.ttl, "test.client_ttl")
			defaultOptions := &options{}
			opt(defaultOptions)
			assert.Equal(t, tt.expected, defaultOptions.clientTTL)
		})
	}
}

func getHostTags() []string {
	return []string{"dogo_state:hungry"}
}

func listsEqual(mustMatch []string) func(candidate []string) bool {
	return func(candidate []string) bool {
		if len(candidate) != len(mustMatch) {
			return false
		}

		candidateSet := make(map[string]struct{})
		for _, item := range candidate {
			candidateSet[item] = struct{}{}
		}

		for _, item := range mustMatch {
			if _, ok := candidateSet[item]; !ok {
				return false
			}
		}

		return true
	}
}

func TestWithOrgStatusPollingIntervalNoConfigPassed(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetWithoutSource("run_path", "/tmp")

	baseRawURL := "https://localhost"
	mockTelemetryReporter := &telemetryReporter{}
	uptaneClient := &mockCoreAgentUptane{}
	options := []Option{
		WithAPIKey("abc"),
		uptaneFactoryOption(uptaneClient),
	}
	service, err := NewService(cfg, "Remote Config", baseRawURL, "localhost", getHostTags, mockTelemetryReporter, agentVersion, options...)
	assert.NoError(t, err)
	assert.Equal(t, service.orgStatusPoller.refreshInterval, defaultRefreshInterval)
	assert.NotNil(t, service)
	service.Stop()
}

func TestWithOrgStatusPollingIntervalConfigPassed(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetWithoutSource("run_path", "/tmp")

	baseRawURL := "https://localhost"
	mockTelemetryReporter := &telemetryReporter{}
	uptaneClient := &mockCoreAgentUptane{}
	options := []Option{
		WithAPIKey("abc"),
		WithOrgStatusRefreshInterval(54*time.Second, "test.org_status_refresh_interval"),
		uptaneFactoryOption(uptaneClient),
	}
	service, err := NewService(cfg, "Remote Config", baseRawURL, "localhost", getHostTags, mockTelemetryReporter, agentVersion, options...)
	assert.NoError(t, err)
	assert.Equal(t, service.orgStatusPoller.refreshInterval, 54*time.Second)
	assert.NotNil(t, service)
	service.Stop()
}

// TestBypassTriggersOnNewProducts verifies that when an already-active client
// adds a new product (e.g., FFE_FLAGS after initially connecting for APM), the
// cache bypass fires. This is the fix for the startup latency issue where the
// bypass only fired for new clients, not new products.
func TestBypassTriggersOnNewProducts(t *testing.T) {
	api := &mockAPI{}
	uptaneClient := &mockCoreAgentUptane{}
	clk := clock.NewMock()
	clk.Set(time.Now())

	service := newTestService(t, api, uptaneClient, clk, WithAgentPollLoopDisabled())

	lastConfigResponse := &pbgo.LatestConfigsResponse{
		TargetFiles: []*pbgo.File{{Path: "test"}},
	}

	uptaneClient.On("StoredOrgUUID").Return("abcdef", nil)
	uptaneClient.On("TUFVersionState").Return(uptane.TUFVersions{}, nil)
	uptaneClient.On("TargetsCustom").Return([]byte{}, nil)
	uptaneClient.On("TimestampExpires").Return(clk.Now().Add(1*time.Hour), nil)
	api.On("Fetch", mock.Anything, mock.Anything).Return(lastConfigResponse, nil)
	uptaneClient.On("Update", lastConfigResponse).Return(nil)

	service.Start()

	// First request: new client with APM_TRACING.
	// This should trigger a bypass (new client).
	clientAPM := &pbgo.Client{
		Id: "tracer-1",
		State: &pbgo.ClientState{
			RootVersion: 1,
		},
		IsTracer: true,
		ClientTracer: &pbgo.ClientTracer{
			RuntimeId: "runtime-1",
			Language:  "go",
		},
		Products: []string{string(rdata.ProductAPMSampling)},
	}
	_, err := service.ClientGetConfigs(context.Background(), &pbgo.ClientGetConfigsRequest{Client: clientAPM})
	require.NoError(t, err)

	// Advance the clock past the 1-second minimum between refreshes
	clk.Add(2 * time.Second)

	// The client is now active. Verify that the same products do NOT trigger a bypass.
	// We check this by counting Fetch calls: there should be exactly 1 from the first bypass.
	fetchCountBefore := countFetchCalls(api)

	clientSameProducts := &pbgo.Client{
		Id: "tracer-1",
		State: &pbgo.ClientState{
			RootVersion: 1,
		},
		IsTracer: true,
		ClientTracer: &pbgo.ClientTracer{
			RuntimeId: "runtime-1",
			Language:  "go",
		},
		Products: []string{string(rdata.ProductAPMSampling)},
	}
	_, err = service.ClientGetConfigs(context.Background(), &pbgo.ClientGetConfigsRequest{Client: clientSameProducts})
	require.NoError(t, err)

	fetchCountAfterSame := countFetchCalls(api)
	assert.Equal(t, fetchCountBefore, fetchCountAfterSame, "same products should NOT trigger bypass")

	// Advance the clock past the 1-second minimum between refreshes
	clk.Add(2 * time.Second)

	// Now the client adds FFE_FLAGS. This should trigger a bypass because
	// the product set changed — even though the client is still active.
	clientWithFFE := &pbgo.Client{
		Id: "tracer-1",
		State: &pbgo.ClientState{
			RootVersion: 1,
		},
		IsTracer: true,
		ClientTracer: &pbgo.ClientTracer{
			RuntimeId: "runtime-1",
			Language:  "go",
		},
		Products: []string{string(rdata.ProductAPMSampling), "FFE_FLAGS"},
	}
	_, err = service.ClientGetConfigs(context.Background(), &pbgo.ClientGetConfigsRequest{Client: clientWithFFE})
	require.NoError(t, err)

	fetchCountAfterFFE := countFetchCalls(api)
	assert.Greater(t, fetchCountAfterFFE, fetchCountAfterSame, "new product FFE_FLAGS should trigger bypass and call Fetch")
}

// countFetchCalls returns the number of times api.Fetch was called.
func countFetchCalls(api *mockAPI) int {
	count := 0
	for _, call := range api.Calls {
		if call.Method == "Fetch" {
			count++
		}
	}
	return count
}
