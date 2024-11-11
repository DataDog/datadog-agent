// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package autodiscoveryimpl

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/listeners"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/scheduler"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	pkglogsetup "github.com/DataDog/datadog-agent/pkg/util/log/setup"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
)

type MockProvider struct {
	collectCounter int
}

//nolint:revive // TODO(AML) Fix revive linter
func (p *MockProvider) Collect(_ context.Context) ([]integration.Config, error) {
	p.collectCounter++
	return []integration.Config{}, nil
}

func (p *MockProvider) String() string {
	return "mocked"
}

//nolint:revive // TODO(AML) Fix revive linter
func (p *MockProvider) IsUpToDate(_ context.Context) (bool, error) {
	return true, nil
}

func (p *MockProvider) GetConfigErrors() map[string]providers.ErrorMsgSet {
	return make(map[string]providers.ErrorMsgSet)
}

type MockProvider2 struct {
	MockProvider
}

type MockListener struct {
	ListenCount  int
	stopReceived bool
}

//nolint:revive // TODO(AML) Fix revive linter
func (l *MockListener) Listen(_, _ chan<- listeners.Service) {
	l.ListenCount++
}

func (l *MockListener) Stop() {
	l.stopReceived = true
}

func (l *MockListener) fakeFactory(listeners.ServiceListernerDeps) (listeners.ServiceListener, error) {
	return l, nil
}

var mockListenenerConfig = pkgconfigsetup.Listeners{
	Name: "mock",
}

type factoryMock struct {
	sync.Mutex
	callCount   int
	callChan    chan struct{}
	returnValue listeners.ServiceListener
	returnError error
}

func (o *factoryMock) make(listeners.ServiceListernerDeps) (listeners.ServiceListener, error) {
	o.Lock()
	defer o.Unlock()
	if o.callChan != nil {
		o.callChan <- struct{}{}
	}
	o.callCount++
	return o.returnValue, o.returnError
}

func (o *factoryMock) waitForCalled(timeout time.Duration) error {
	select {
	case <-o.callChan:
		return nil
	case <-time.After(timeout):
		return errors.New("timeout while waiting for call")
	}
}

func (o *factoryMock) assertCallNumber(t *testing.T, calls int) bool {
	o.Lock()
	defer o.Unlock()
	return assert.Equal(t, calls, o.callCount)
}

func (o *factoryMock) resetCallChan() {
	for {
		select {
		case <-o.callChan:
			continue
		default:
			return
		}
	}
}

type MockScheduler struct {
	scheduled map[string]integration.Config
	mutex     sync.RWMutex
}

// Schedule implements scheduler.Scheduler#Schedule.
func (ms *MockScheduler) Schedule(configs []integration.Config) {
	ms.mutex.Lock()
	defer ms.mutex.Unlock()
	for _, cfg := range configs {
		ms.scheduled[cfg.Digest()] = cfg
	}
}

// Unchedule implements scheduler.Scheduler#Unchedule.
func (ms *MockScheduler) Unschedule(configs []integration.Config) {
	ms.mutex.Lock()
	defer ms.mutex.Unlock()
	for _, cfg := range configs {
		delete(ms.scheduled, cfg.Digest())
	}
}

// Stop implements scheduler.Scheduler#Stop.
func (ms *MockScheduler) Stop() {}

func (ms *MockScheduler) scheduledSize() int {
	ms.mutex.Lock()
	defer ms.mutex.Unlock()
	return len(ms.scheduled)
}

type AutoConfigTestSuite struct {
	suite.Suite
	deps Deps
}

// SetupSuite saves the original listener registry
func (suite *AutoConfigTestSuite) SetupSuite() {
	pkglogsetup.SetupLogger(
		pkglogsetup.LoggerName("test"),
		"debug",
		"",
		"",
		false,
		true,
		false,
		pkgconfigsetup.Datadog(),
	)
}

func (suite *AutoConfigTestSuite) SetupTest() {
	suite.deps = createDeps(suite.T())
}

func getAutoConfig(schedulerController *scheduler.Controller, secretResolver secrets.Component, wmeta optional.Option[workloadmeta.Component], taggerComp tagger.Component, logsComp log.Component, telemetryComp telemetry.Component) *AutoConfig {
	ac := createNewAutoConfig(schedulerController, secretResolver, wmeta, taggerComp, logsComp, telemetryComp)
	go ac.serviceListening()
	return ac
}

func (suite *AutoConfigTestSuite) TestAddConfigProvider() {
	mockResolver := MockSecretResolver{suite.T(), nil}
	ac := getAutoConfig(scheduler.NewController(), &mockResolver, suite.deps.WMeta, suite.deps.TaggerComp, suite.deps.LogsComp, suite.deps.Telemetry)
	assert.Len(suite.T(), ac.configPollers, 0)
	mp := &MockProvider{}
	ac.AddConfigProvider(mp, false, 0)
	ac.AddConfigProvider(&MockProvider2{}, true, 1*time.Second)

	require.Len(suite.T(), ac.configPollers, 2)
	assert.False(suite.T(), ac.configPollers[0].canPoll)
	assert.True(suite.T(), ac.configPollers[1].canPoll)

	ac.LoadAndRun(context.Background())

	assert.Equal(suite.T(), 1, mp.collectCounter)
}

func (suite *AutoConfigTestSuite) TestAddListener() {
	mockResolver := MockSecretResolver{suite.T(), nil}
	ac := getAutoConfig(scheduler.NewController(), &mockResolver, suite.deps.WMeta, suite.deps.TaggerComp, suite.deps.LogsComp, suite.deps.Telemetry)
	assert.Len(suite.T(), ac.listeners, 0)

	ml := &MockListener{}
	listeners.Register("mock", ml.fakeFactory, ac.serviceListenerFactories)
	ac.AddListeners([]pkgconfigsetup.Listeners{mockListenenerConfig})

	ac.m.Lock()
	require.Len(suite.T(), ac.listeners, 1)
	assert.Equal(suite.T(), 1, ml.ListenCount)
	// Retry goroutine should be started
	assert.Nil(suite.T(), ac.listenerRetryStop)
	assert.Len(suite.T(), ac.listenerCandidates, 0)
	ac.m.Unlock()
}

func (suite *AutoConfigTestSuite) TestDiffConfigs() {
	c1 := integration.Config{Name: "bar"}
	c2 := integration.Config{Name: "foo"}
	c3 := integration.Config{Name: "baz"}
	pd := configPoller{}

	pd.configs = map[uint64]integration.Config{
		c1.FastDigest(): c1,
		c2.FastDigest(): c2,
	}

	added, removed := pd.storeAndDiffConfigs([]integration.Config{c1, c3})
	assert.ElementsMatch(suite.T(), added, []integration.Config{c3})
	assert.ElementsMatch(suite.T(), removed, []integration.Config{c2})
	assert.Equal(suite.T(), map[uint64]integration.Config{
		c3.FastDigest(): c3,
		c1.FastDigest(): c1,
	}, pd.configs)
}

func (suite *AutoConfigTestSuite) TestStop() {
	mockResolver := MockSecretResolver{suite.T(), nil}
	ac := getAutoConfig(scheduler.NewController(), &mockResolver, suite.deps.WMeta, suite.deps.TaggerComp, suite.deps.LogsComp, suite.deps.Telemetry)

	ml := &MockListener{}
	listeners.Register("mock", ml.fakeFactory, ac.serviceListenerFactories)
	ac.AddListeners([]pkgconfigsetup.Listeners{mockListenenerConfig})

	ac.Stop()

	assert.True(suite.T(), ml.stopReceived)
}

func (suite *AutoConfigTestSuite) TestListenerRetry() {
	mockResolver := MockSecretResolver{suite.T(), nil}
	ac := getAutoConfig(scheduler.NewController(), &mockResolver, suite.deps.WMeta, suite.deps.TaggerComp, suite.deps.LogsComp, suite.deps.Telemetry)

	// Hack the retry delay to shorten the test run time
	initialListenerCandidateIntl := listenerCandidateIntl
	listenerCandidateIntl = 50 * time.Millisecond
	defer func() { listenerCandidateIntl = initialListenerCandidateIntl }()

	// noErrFactory succeeds on first try
	noErrListener := &MockListener{}
	noErrFactory := factoryMock{
		returnError: nil,
		returnValue: noErrListener,
	}
	listeners.Register("noerr", noErrFactory.make, ac.serviceListenerFactories)

	// failFactory does not implement retry, should be discarded on first fail
	failListener := &MockListener{}
	failFactory := factoryMock{
		returnError: errors.New("permafail"),
		returnValue: failListener,
	}
	listeners.Register("fail", failFactory.make, ac.serviceListenerFactories)

	// retryFactory implements retry
	retryListener := &MockListener{}
	retryFactory := factoryMock{
		callChan: make(chan struct{}, 3),
		returnError: &retry.Error{
			LogicError:    errors.New("will retry"),
			RessourceName: "mocked",
			RetryStatus:   retry.FailWillRetry,
		},
		returnValue: retryListener,
	}
	listeners.Register("retry", retryFactory.make, ac.serviceListenerFactories)

	configs := []pkgconfigsetup.Listeners{
		{Name: "noerr"},
		{Name: "fail"},
		{Name: "retry"},
		{Name: "invalid"},
	}
	assert.Nil(suite.T(), ac.listenerRetryStop)
	ac.AddListeners(configs)

	ac.m.Lock()
	// First try is synchronous, all factories should be called
	noErrFactory.assertCallNumber(suite.T(), 1)
	failFactory.assertCallNumber(suite.T(), 1)
	retryFactory.assertCallNumber(suite.T(), 1)
	// We should keep a single candidate
	assert.Len(suite.T(), ac.listenerCandidates, 1)
	assert.NotNil(suite.T(), ac.listenerCandidates["retry"])
	// Listen should be called on the noErrListener, not on the other ones
	assert.Equal(suite.T(), 1, noErrListener.ListenCount)
	assert.Equal(suite.T(), 0, failListener.ListenCount)
	assert.Equal(suite.T(), 0, retryListener.ListenCount)
	// Retry goroutine should be started
	assert.NotNil(suite.T(), ac.listenerRetryStop)
	ac.m.Unlock()

	// Second failure of the retryFactory
	retryFactory.resetCallChan()
	assert.Eventually(suite.T(), func() bool {
		retryFactory.Lock()
		defer retryFactory.Unlock()
		return retryFactory.callCount >= 2
	}, 2*time.Second, 10*time.Millisecond)
	assert.Equal(suite.T(), 0, retryListener.ListenCount)
	// failFactory should not be called again
	failFactory.assertCallNumber(suite.T(), 1)

	// Make retryFactory successful now
	retryFactory.Lock()
	retryFactory.returnError = nil
	retryFactory.resetCallChan()
	retryFactory.Unlock()
	err := retryFactory.waitForCalled(500 * time.Millisecond)
	assert.NoError(suite.T(), err)

	// Lock to wait for initListenerCandidates to return
	// We should start retryListener and have no more candidate
	ac.m.Lock()
	assert.Equal(suite.T(), 1, retryListener.ListenCount)
	assert.Len(suite.T(), ac.listenerCandidates, 0)
	ac.m.Unlock()

	// Wait for retryListenerCandidates to close listenerRetryStop and return
	assert.Eventually(suite.T(), func() bool {
		ac.m.Lock()
		defer ac.m.Unlock()
		return ac.listenerRetryStop == nil
	}, 2*time.Second, 10*time.Millisecond)
}

func TestAutoConfigTestSuite(t *testing.T) {
	suite.Run(t, new(AutoConfigTestSuite))
}

func TestResolveTemplate(t *testing.T) {
	deps := createDeps(t)
	ctx := context.Background()

	msch := scheduler.NewController()
	sch := &MockScheduler{scheduled: make(map[string]integration.Config)}
	msch.Register("mock", sch, false)

	mockResolver := MockSecretResolver{t, nil}
	ac := getAutoConfig(msch, &mockResolver, deps.WMeta, deps.TaggerComp, deps.LogsComp, deps.Telemetry)
	tpl := integration.Config{
		Name:          "cpu",
		ADIdentifiers: []string{"redis"},
	}

	// no services
	changes := ac.processNewConfig(tpl)
	ac.applyChanges(changes) // processNewConfigs does not apply changes

	assert.Equal(t, sch.scheduledSize(), 0)

	service := dummyService{
		ID:            "a5901276aed16ae9ea11660a41fecd674da47e8f5d8d5bce0080a611feed2be9",
		ADIdentifiers: []string{"redis"},
	}
	// there are no template vars but it's ok
	ac.processNewService(ctx, &service) // processNewService applies changes
	assert.Eventually(t, func() bool {
		return sch.scheduledSize() == 1
	}, 5*time.Second, 10*time.Millisecond)
}

func countLoadedConfigs(ac *AutoConfig) int {
	count := -1 // -1 would indicate f was not called
	ac.MapOverLoadedConfigs(func(loadedConfigs map[string]integration.Config) {
		count = len(loadedConfigs)
	})
	return count
}

func TestRemoveTemplate(t *testing.T) {
	deps := createDeps(t)
	ctx := context.Background()

	mockResolver := MockSecretResolver{t, nil}

	ac := getAutoConfig(scheduler.NewController(), &mockResolver, deps.WMeta, deps.TaggerComp, deps.LogsComp, deps.Telemetry)
	// Add static config
	c := integration.Config{
		Name: "memory",
	}
	ac.processNewConfig(c)
	assert.Equal(t, countLoadedConfigs(ac), 1)

	// Add new service
	service := dummyService{
		ID:            "a5901276aed16ae9ea11660a41fecd674da47e8f5d8d5bce0080a611feed2be9",
		ADIdentifiers: []string{"redis"},
	}
	ac.processNewService(ctx, &service)

	// Add matching template
	tpl := integration.Config{
		Name:          "cpu",
		ADIdentifiers: []string{"redis"},
	}
	changes := ac.processNewConfig(tpl)
	assert.Len(t, changes.Schedule, 1)
	assert.Equal(t, countLoadedConfigs(ac), 2)

	// Remove the template, config should be removed too
	ac.processRemovedConfigs([]integration.Config{tpl})
	assert.Equal(t, countLoadedConfigs(ac), 1)
}

func TestGetLoadedConfigNotInitialized(t *testing.T) {
	ac := AutoConfig{}
	assert.Equal(t, countLoadedConfigs(&ac), 0)
}

func TestDecryptConfig(t *testing.T) {
	deps := createDeps(t)
	ctx := context.Background()

	mockResolver := MockSecretResolver{t, []mockSecretScenario{
		{
			expectedData:   []byte{},
			expectedOrigin: "cpu",
			returnedData:   []byte{},
			returnedError:  nil,
		},
		{
			expectedData:   []byte("param1: ENC[foo]\n"),
			expectedOrigin: "cpu",
			returnedData:   []byte("param1: foo\n"),
			returnedError:  nil,
		},
	}}

	ac := getAutoConfig(scheduler.NewController(), &mockResolver, deps.WMeta, deps.TaggerComp, deps.LogsComp, deps.Telemetry)
	ac.processNewService(ctx, &dummyService{ID: "abcd", ADIdentifiers: []string{"redis"}})

	tpl := integration.Config{
		Name:          "cpu",
		ADIdentifiers: []string{"redis"},
		InitConfig:    []byte("param1: ENC[foo]"),
	}
	changes := ac.processNewConfig(tpl)

	require.Len(t, changes.Schedule, 1)

	resolved := integration.Config{
		Name:          "cpu",
		ADIdentifiers: []string{"redis"},
		InitConfig:    []byte("param1: foo\n"),
		Instances:     []integration.Data{},
		MetricConfig:  integration.Data{},
		LogsConfig:    integration.Data{},
		ServiceID:     "abcd",
	}
	assert.Equal(t, resolved, changes.Schedule[0])

	assert.True(t, mockResolver.haveAllScenariosBeenCalled())
}

func TestProcessClusterCheckConfigWithSecrets(t *testing.T) {
	deps := createDeps(t)
	configName := "testConfig"

	mockResolver := MockSecretResolver{t, []mockSecretScenario{
		{
			expectedData:   []byte("foo: ENC[bar]"),
			expectedOrigin: configName,
			returnedData:   []byte("foo: barDecoded"),
			returnedError:  nil,
		},
		{
			expectedData:   []byte{},
			expectedOrigin: configName,
			returnedData:   []byte{},
			returnedError:  nil,
		},
	}}
	ac := getAutoConfig(scheduler.NewController(), &mockResolver, deps.WMeta, deps.TaggerComp, deps.LogsComp, deps.Telemetry)

	tpl := integration.Config{
		Provider:     names.ClusterChecks,
		Name:         configName,
		InitConfig:   integration.Data{},
		Instances:    []integration.Data{integration.Data("foo: ENC[bar]")},
		MetricConfig: integration.Data{},
		LogsConfig:   integration.Data{},
	}
	changes := ac.processNewConfig(tpl)

	require.Len(t, changes.Schedule, 1)

	resolved := integration.Config{
		Provider:     names.ClusterChecks,
		Name:         configName,
		InitConfig:   integration.Data{},
		Instances:    []integration.Data{integration.Data("foo: barDecoded")},
		MetricConfig: integration.Data{},
		LogsConfig:   integration.Data{},
	}
	assert.Equal(t, resolved, changes.Schedule[0])

	// Check that the mapping with the changeIDs is stored
	originalCheckID := checkid.BuildID(tpl.Name, tpl.FastDigest(), tpl.Instances[0], tpl.InitConfig)
	newCheckID := checkid.BuildID(resolved.Name, resolved.FastDigest(), resolved.Instances[0], resolved.InitConfig)
	assert.Equal(t, originalCheckID, ac.GetIDOfCheckWithEncryptedSecrets(newCheckID))
}

func TestWriteConfigEndpoint(t *testing.T) {
	deps := createDeps(t)
	configName := "testConfig"

	mockResolver := MockSecretResolver{t, nil}
	ac := getAutoConfig(scheduler.NewController(), &mockResolver, deps.WMeta, deps.TaggerComp, deps.LogsComp, deps.Telemetry)

	tpl := integration.Config{
		Provider:     names.ClusterChecks,
		Name:         configName,
		InitConfig:   integration.Data{},
		Instances:    []integration.Data{integration.Data("pass: 1234567")},
		MetricConfig: integration.Data{},
		LogsConfig:   integration.Data{},
	}
	changes := ac.processNewConfig(tpl)

	require.Len(t, changes.Schedule, 1)

	testCases := []struct {
		name           string
		request        *http.Request
		expectedResult string
	}{
		{
			name:           "With configuration scrubbed",
			request:        httptest.NewRequest("GET", "http://example.com", nil),
			expectedResult: "pass: \"********\"",
		},
		{
			name:           "With nil Requet",
			request:        nil,
			expectedResult: "pass: \"********\"",
		},
		{
			name:           "Without scrubbing configuration",
			request:        httptest.NewRequest("GET", "http://example.com?raw=true", nil),
			expectedResult: "pass: 1234567",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			responseRecorder := httptest.NewRecorder()
			ac.writeConfigCheck(responseRecorder, tc.request)
			var result integration.ConfigCheckResponse
			out := responseRecorder.Body.Bytes()
			err := json.Unmarshal(out, &result)
			require.NoError(t, err)
			assert.Equal(t, string(result.Configs[0].Instances[0]), tc.expectedResult)
		})
	}
}

type Deps struct {
	fx.In
	WMeta      optional.Option[workloadmeta.Component]
	TaggerComp tagger.Component
	LogsComp   log.Component
	Telemetry  telemetry.Component
}

func createDeps(t *testing.T) Deps {
	return fxutil.Test[Deps](t, core.MockBundle(), workloadmetafxmock.MockModule(workloadmeta.NewParams()), fx.Supply(tagger.NewFakeTaggerParams()), taggerimpl.Module())
}
