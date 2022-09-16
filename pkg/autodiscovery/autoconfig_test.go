// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package autodiscovery

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/listeners"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/providers"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/scheduler"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
)

type MockProvider struct {
	collectCounter int
}

func (p *MockProvider) Collect(ctx context.Context) ([]integration.Config, error) {
	p.collectCounter++
	return []integration.Config{}, nil
}

func (p *MockProvider) String() string {
	return "mocked"
}

func (p *MockProvider) IsUpToDate(ctx context.Context) (bool, error) {
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

func (l *MockListener) Listen(newSvc, delSvc chan<- listeners.Service) {
	l.ListenCount++
}

func (l *MockListener) Stop() {
	l.stopReceived = true
}

func (l *MockListener) fakeFactory(listeners.Config) (listeners.ServiceListener, error) {
	return l, nil
}

var mockListenenerConfig = config.Listeners{
	Name: "mock",
}

type factoryMock struct {
	sync.Mutex
	callCount   int
	callChan    chan struct{}
	returnValue listeners.ServiceListener
	returnError error
}

func (o *factoryMock) make(listeners.Config) (listeners.ServiceListener, error) {
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
}

// Schedule implements scheduler.Scheduler#Schedule.
func (ms *MockScheduler) Schedule(configs []integration.Config) {
	for _, cfg := range configs {
		ms.scheduled[cfg.Digest()] = cfg
	}
}

// Unchedule implements scheduler.Scheduler#Unchedule.
func (ms *MockScheduler) Unschedule(configs []integration.Config) {
	for _, cfg := range configs {
		delete(ms.scheduled, cfg.Digest())
	}
}

// Stop implements scheduler.Scheduler#Stop.
func (ms *MockScheduler) Stop() {}

type AutoConfigTestSuite struct {
	suite.Suite
	originalListeners map[string]listeners.ServiceListenerFactory
}

// SetupSuite saves the original listener registry
func (suite *AutoConfigTestSuite) SetupSuite() {
	suite.originalListeners = listeners.ServiceListenerFactories
	config.SetupLogger(
		config.LoggerName("test"),
		"debug",
		"",
		"",
		false,
		true,
		false,
	)
}

// TearDownSuite restores the original listener registry
func (suite *AutoConfigTestSuite) TearDownSuite() {
	listeners.ServiceListenerFactories = suite.originalListeners
}

// Empty the listener registry before each test
func (suite *AutoConfigTestSuite) SetupTest() {
	listeners.ServiceListenerFactories = make(map[string]listeners.ServiceListenerFactory)
}

func (suite *AutoConfigTestSuite) TestAddConfigProvider() {
	ac := NewAutoConfig(scheduler.NewMetaScheduler())
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
	ac := NewAutoConfig(scheduler.NewMetaScheduler())
	assert.Len(suite.T(), ac.listeners, 0)

	ml := &MockListener{}
	listeners.Register("mock", ml.fakeFactory)
	ac.AddListeners([]config.Listeners{mockListenenerConfig})

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
	ac := NewAutoConfig(scheduler.NewMetaScheduler())

	ml := &MockListener{}
	listeners.Register("mock", ml.fakeFactory)
	ac.AddListeners([]config.Listeners{mockListenenerConfig})

	ac.Stop()

	assert.True(suite.T(), ml.stopReceived)
}

func (suite *AutoConfigTestSuite) TestListenerRetry() {
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
	listeners.Register("noerr", noErrFactory.make)

	// failFactory does not implement retry, should be discarded on first fail
	failListener := &MockListener{}
	failFactory := factoryMock{
		returnError: errors.New("permafail"),
		returnValue: failListener,
	}
	listeners.Register("fail", failFactory.make)

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
	listeners.Register("retry", retryFactory.make)

	configs := []config.Listeners{
		{Name: "noerr"},
		{Name: "fail"},
		{Name: "retry"},
		{Name: "invalid"},
	}
	ac := NewAutoConfig(scheduler.NewMetaScheduler())
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
	err := retryFactory.waitForCalled(500 * time.Millisecond)
	assert.NoError(suite.T(), err)
	retryFactory.assertCallNumber(suite.T(), 2)
	assert.Equal(suite.T(), 0, retryListener.ListenCount)
	// failFactory should not be called again
	failFactory.assertCallNumber(suite.T(), 1)

	// Make retryFactory successful now
	retryFactory.Lock()
	retryFactory.returnError = nil
	retryFactory.resetCallChan()
	retryFactory.Unlock()
	err = retryFactory.waitForCalled(500 * time.Millisecond)
	assert.NoError(suite.T(), err)

	// Lock to wait for initListenerCandidates to return
	// We should start retryListener and have no more candidate
	ac.m.Lock()
	retryFactory.assertCallNumber(suite.T(), 3)
	assert.Equal(suite.T(), 1, retryListener.ListenCount)
	assert.Len(suite.T(), ac.listenerCandidates, 0)
	ac.m.Unlock()

	// Wait for retryListenerCandidates to close listenerRetryStop and return
	for i := 0; i < 10; i++ {
		ac.m.Lock()
		nilled := (ac.listenerRetryStop == nil)
		ac.m.Unlock()
		if nilled {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	ac.m.Lock()
	assert.Nil(suite.T(), ac.listenerRetryStop)
	ac.m.Unlock()
}

func TestAutoConfigTestSuite(t *testing.T) {
	suite.Run(t, new(AutoConfigTestSuite))
}

func TestResolveTemplate(t *testing.T) {
	ctx := context.Background()

	msch := scheduler.NewMetaScheduler()
	sch := &MockScheduler{scheduled: make(map[string]integration.Config)}
	msch.Register("mock", sch, false)

	ac := NewAutoConfig(msch)
	tpl := integration.Config{
		Name:          "cpu",
		ADIdentifiers: []string{"redis"},
	}

	// no services
	changes := ac.processNewConfig(tpl)
	ac.applyChanges(changes) // processNewConfigs does not apply changes

	assert.Len(t, sch.scheduled, 0)

	service := dummyService{
		ID:            "a5901276aed16ae9ea11660a41fecd674da47e8f5d8d5bce0080a611feed2be9",
		ADIdentifiers: []string{"redis"},
	}
	// there are no template vars but it's ok
	ac.processNewService(ctx, &service) // processNewService applies changes

	assert.Len(t, sch.scheduled, 1)
}

func countLoadedConfigs(ac *AutoConfig) int {
	count := -1 // -1 would indicate f was not called
	ac.MapOverLoadedConfigs(func(loadedConfigs map[string]integration.Config) {
		count = len(loadedConfigs)
	})
	return count
}

func TestRemoveTemplate(t *testing.T) {
	ctx := context.Background()

	ac := NewAutoConfig(scheduler.NewMetaScheduler())

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
	ctx := context.Background()

	mockDecrypt := MockSecretDecrypt{t, []mockSecretScenario{
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
	defer mockDecrypt.install()()

	ac := NewAutoConfig(scheduler.NewMetaScheduler())
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

	assert.True(t, mockDecrypt.haveAllScenariosBeenCalled())
}
