// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks && kubeapiserver && test

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
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/utils"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/listeners"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/scheduler"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadfilterfxmock "github.com/DataDog/datadog-agent/comp/core/workloadfilter/fx-mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	pkglogsetup "github.com/DataDog/datadog-agent/pkg/util/log/setup"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
)

type MockProvider struct {
	collectCounter int
}

func (p *MockProvider) Collect(_ context.Context) ([]integration.Config, error) {
	p.collectCounter++
	return []integration.Config{}, nil
}

func (p *MockProvider) String() string {
	return "mocked"
}

func (p *MockProvider) IsUpToDate(_ context.Context) (bool, error) {
	return true, nil
}

func (p *MockProvider) GetConfigErrors() map[string]types.ErrorMsgSet {
	return make(map[string]types.ErrorMsgSet)
}

type MockProvider2 struct {
	MockProvider
}

type MockListener struct {
	ListenCount  int
	stopReceived bool
}

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

func (ms *MockScheduler) isScheduled(digest string) bool {
	ms.mutex.RLock()
	defer ms.mutex.RUnlock()
	_, found := ms.scheduled[digest]
	return found
}

type AutoConfigTestSuite struct {
	suite.Suite
	deps Deps
}

// SetupSuite saves the original listener registry
func (suite *AutoConfigTestSuite) SetupSuite() {
	cfg := configmock.New(suite.T())
	pkglogsetup.SetupLogger(
		pkglogsetup.LoggerName("test"),
		"debug",
		"",
		"",
		false,
		true,
		false,
		cfg,
	)
}

func (suite *AutoConfigTestSuite) SetupTest() {
	suite.deps = createDeps(suite.T())
}

func getAutoConfig(schedulerController *scheduler.Controller, secretResolver secrets.Component, wmeta option.Option[workloadmeta.Component], taggerComp tagger.Component, logsComp log.Component, telemetryComp telemetry.Component, filterComp workloadfilter.Component) *AutoConfig {
	ac := createNewAutoConfig(schedulerController, secretResolver, wmeta, taggerComp, logsComp, telemetryComp, filterComp)
	go ac.serviceListening()
	return ac
}

func (suite *AutoConfigTestSuite) TestAddConfigProvider() {
	mockResolver := MockSecretResolver{t: suite.T(), scenarios: nil}
	ac := getAutoConfig(scheduler.NewControllerAndStart(), &mockResolver, suite.deps.WMeta, suite.deps.TaggerComp, suite.deps.LogsComp, suite.deps.Telemetry, suite.deps.FilterComp)
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
	mockResolver := MockSecretResolver{t: suite.T(), scenarios: nil}
	ac := getAutoConfig(scheduler.NewControllerAndStart(), &mockResolver, suite.deps.WMeta, suite.deps.TaggerComp, suite.deps.LogsComp, suite.deps.Telemetry, suite.deps.FilterComp)
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
	mockResolver := MockSecretResolver{t: suite.T(), scenarios: nil}
	ac := getAutoConfig(scheduler.NewControllerAndStart(), &mockResolver, suite.deps.WMeta, suite.deps.TaggerComp, suite.deps.LogsComp, suite.deps.Telemetry, suite.deps.FilterComp)

	ml := &MockListener{}
	listeners.Register("mock", ml.fakeFactory, ac.serviceListenerFactories)
	ac.AddListeners([]pkgconfigsetup.Listeners{mockListenenerConfig})

	ac.stop()

	assert.True(suite.T(), ml.stopReceived)
}

func (suite *AutoConfigTestSuite) TestListenerRetry() {
	mockResolver := MockSecretResolver{t: suite.T(), scenarios: nil}
	ac := getAutoConfig(scheduler.NewControllerAndStart(), &mockResolver, suite.deps.WMeta, suite.deps.TaggerComp, suite.deps.LogsComp, suite.deps.Telemetry, suite.deps.FilterComp)

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

func getResolveTestConfig(t *testing.T) (*MockScheduler, *AutoConfig) {
	deps := createDeps(t)

	msch := scheduler.NewControllerAndStart()
	sch := &MockScheduler{scheduled: make(map[string]integration.Config)}
	msch.Register("mock", sch, false)

	mockResolver := MockSecretResolver{t: t, scenarios: nil}
	ac := getAutoConfig(msch, &mockResolver, deps.WMeta, deps.TaggerComp, deps.LogsComp, deps.Telemetry, deps.FilterComp)

	return sch, ac
}

func TestResolveTemplate(t *testing.T) {
	mockTagger := taggerfxmock.SetupFakeTagger(t)
	mockStore := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	t.Run("AD Identifier", func(t *testing.T) {
		sch, ac := getResolveTestConfig(t)

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
		ac.processNewService(&service) // processNewService applies changes
		assert.Eventually(t, func() bool {
			return sch.scheduledSize() == 1
		}, 5*time.Second, 10*time.Millisecond)
	})

	t.Run("CEL Identifier on Kubernetes Service", func(t *testing.T) {
		_, ac := getResolveTestConfig(t)

		tpl := integration.Config{
			Name:        "service-check",
			CELSelector: workloadfilter.Rules{KubeServices: []string{`kube_service.name.matches("redis") && kube_service.namespace == "default"`}},
		}
		changes := ac.processNewConfig(tpl)
		ac.applyChanges(changes)
		assert.Equal(t, 0, countLoadedConfigs(ac))

		// Test matching services
		matchingService := listeners.CreateDummyKubeService("redis-service", "default", map[string]string{})
		ac.processNewService(matchingService)
		assert.Equal(t, 1, countLoadedConfigs(ac))

		// Test non-matching services
		service := listeners.CreateDummyKubeService("other-service", "default", map[string]string{})
		ac.processNewService(service)
		assert.Equal(t, 1, countLoadedConfigs(ac))

		// Test service deletion
		ac.processDelService(service)
		assert.Equal(t, 1, countLoadedConfigs(ac))

		ac.processDelService(matchingService)
		assert.Equal(t, 0, countLoadedConfigs(ac))
	})

	t.Run("CEL Identifier on Kubernetes Endpoint", func(t *testing.T) {
		_, ac := getResolveTestConfig(t)

		tpl := integration.Config{
			Name:        "endpoint-check",
			CELSelector: workloadfilter.Rules{KubeEndpoints: []string{`kube_endpoint.namespace.matches("include-ns") && !kube_endpoint.name.matches("exclude-name") && !("team" in kube_endpoint.annotations && kube_endpoint.annotations["team"].matches("exclude"))`}},
		}
		changes := ac.processNewConfig(tpl)
		ac.applyChanges(changes)
		assert.Equal(t, 0, countLoadedConfigs(ac))

		// Test matching endpoints
		matchingService := listeners.CreateDummyKubeEndpoint("name", "include-ns", map[string]string{})
		ac.processNewService(matchingService)
		assert.Equal(t, 1, countLoadedConfigs(ac))

		// Test non-matching endpoints
		service := listeners.CreateDummyKubeEndpoint("name", "default", map[string]string{})
		ac.processNewService(service)
		assert.Equal(t, 1, countLoadedConfigs(ac))

		service = listeners.CreateDummyKubeEndpoint("exclude-name", "include-ns", map[string]string{})
		ac.processNewService(service)
		assert.Equal(t, 1, countLoadedConfigs(ac))

		service = listeners.CreateDummyKubeEndpoint("name", "include-ns", map[string]string{"team": "exclude"})
		ac.processNewService(service)
		assert.Equal(t, 1, countLoadedConfigs(ac))

		// Test endpoint deletion
		ac.processDelService(matchingService)
		assert.Equal(t, 0, countLoadedConfigs(ac))
	})

	t.Run("CEL Identifier on Container", func(t *testing.T) {
		mockConfig := configmock.New(t)
		mockConfig.SetWithoutSource("logs_config.container_collect_all", true)

		// Setup container tied to a pod
		wmetaPod := listeners.CreateDummyPod("pod-name", "pod-ns", nil)
		wmetaCtn := listeners.CreateDummyContainer("container-name", "container-image")
		wmetaCtn.Owner = &wmetaPod.EntityID
		mockStore.Set(wmetaPod)
		mockStore.Set(wmetaCtn)

		_, ac := getResolveTestConfig(t)

		service := listeners.CreateDummyContainerService(wmetaCtn, mockTagger, mockStore)
		ac.processNewService(service)
		assert.Equal(t, 0, countLoadedConfigs(ac))

		// Container name and image matching
		tpl := integration.Config{
			Name:        "container-check-1",
			CELSelector: workloadfilter.Rules{Containers: []string{`container.name.matches("container-name") && container.image.reference.matches("container-image")`}},
		}
		changes := ac.processNewConfig(tpl)
		ac.applyChanges(changes)
		assert.Equal(t, 1, countLoadedConfigs(ac))

		// Pod name and namespace matching
		tpl = integration.Config{
			Name:        "container-check-2",
			CELSelector: workloadfilter.Rules{Containers: []string{`container.pod.name.matches("pod-name") && container.pod.namespace.matches("pod-ns") && container.image.reference != ""`}},
		}
		ac.applyChanges(ac.processNewConfig(tpl))
		assert.Equal(t, 2, countLoadedConfigs(ac))

		// CCA
		tpl = utils.AddContainerCollectAllConfigs([]integration.Config{}, "container-image")[0]
		ac.applyChanges(ac.processNewConfig(tpl))
		assert.Equal(t, 3, countLoadedConfigs(ac))
		activeConfigs := ac.GetAllConfigs()
		activeConfigNames := make([]string, 0, len(activeConfigs))
		for _, cfg := range activeConfigs {
			activeConfigNames = append(activeConfigNames, cfg.Name)
		}
		expectedConfigNames := []string{"container-check-1", "container-check-2", "container_collect_all"}
		assert.ElementsMatch(t, expectedConfigNames, activeConfigNames)

		// AD Identifier + CEL matching
		tpl = integration.Config{
			Name:          "container-check-3",
			ADIdentifiers: []string{"container-image"},
			LogsConfig:    []byte(`{"source":"test-source"}`),
			CELSelector:   workloadfilter.Rules{Containers: []string{`container.pod.name.matches("pod-name")`}},
		}
		logsConfigChanges := ac.processNewConfig(tpl)
		ac.applyChanges(logsConfigChanges)
		assert.Equal(t, 3, countLoadedConfigs(ac))

		// Should override the generic container_collect_all check config
		activeConfigs = ac.GetAllConfigs()
		for _, cfg := range activeConfigs {
			assert.NotEqual(t, "container_collect_all", cfg.Name)
		}

		// Bad AD Identifier + CEL matching
		tpl = integration.Config{
			Name:          "container-check-4",
			ADIdentifiers: []string{"not-container-image"},
			CELSelector:   workloadfilter.Rules{Containers: []string{`container.pod.name.matches("pod-name") && container.image.reference != ""`}},
		}
		ac.applyChanges(ac.processNewConfig(tpl))
		assert.Equal(t, 3, countLoadedConfigs(ac))

		// Test config deletion
		ac.processRemovedConfigs(changes.Schedule)
		assert.Equal(t, 2, countLoadedConfigs(ac))

		// Test service deletion
		ac.processDelService(service)
		assert.Equal(t, 0, countLoadedConfigs(ac))
	})
}

func countLoadedConfigs(ac *AutoConfig) int {
	if ac == nil || ac.store == nil {
		return 0
	}
	return len(ac.GetAllConfigs())
}

func TestGetUnresolvedConfigs(t *testing.T) {
	deps := createDeps(t)
	c := integration.Config{
		Name:       "kafka",
		InitConfig: []byte("param1: ENC[foo]\n"),
	}
	mockResolver := MockSecretResolver{t: t, scenarios: []mockSecretScenario{
		{
			expectedData:   []byte{},
			expectedOrigin: c.Digest(),
			returnedData:   []byte{},
			returnedError:  nil,
		},
		{
			expectedData:   []byte("param1: ENC[foo]\n"),
			expectedOrigin: c.Digest(),
			returnedData:   []byte("param1: foo\n"),
			returnedError:  nil,
		},
	}}
	ac := getAutoConfig(scheduler.NewControllerAndStart(), &mockResolver, deps.WMeta, deps.TaggerComp, deps.LogsComp, deps.Telemetry, deps.FilterComp)
	ac.processNewConfig(c)
	assert.Equal(t, []integration.Config{c}, ac.GetUnresolvedConfigs())
	assert.Equal(t, []integration.Config{{
		Name:         "kafka",
		Instances:    []integration.Data{},
		InitConfig:   []byte("param1: foo\n"),
		MetricConfig: integration.Data{},
		LogsConfig:   integration.Data{},
	}}, ac.GetAllConfigs())
}

func TestRemoveTemplate(t *testing.T) {
	deps := createDeps(t)

	mockResolver := MockSecretResolver{t: t, scenarios: nil}

	ac := getAutoConfig(scheduler.NewControllerAndStart(), &mockResolver, deps.WMeta, deps.TaggerComp, deps.LogsComp, deps.Telemetry, deps.FilterComp)
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
	ac.processNewService(&service)

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

	tpl := integration.Config{
		Name:          "cpu",
		ADIdentifiers: []string{"redis"},
		InitConfig:    []byte("param1: ENC[foo]"),
	}
	mockResolver := MockSecretResolver{t: t, scenarios: []mockSecretScenario{
		{
			expectedData:   []byte{},
			expectedOrigin: tpl.Digest(),
			returnedData:   []byte{},
			returnedError:  nil,
		},
		{
			expectedData:   []byte("param1: ENC[foo]\n"),
			expectedOrigin: tpl.Digest(),
			returnedData:   []byte("param1: foo\n"),
			returnedError:  nil,
		},
	}}

	ac := getAutoConfig(scheduler.NewControllerAndStart(), &mockResolver, deps.WMeta, deps.TaggerComp, deps.LogsComp, deps.Telemetry, deps.FilterComp)
	ac.processNewService(&dummyService{ID: "abcd", ADIdentifiers: []string{"redis"}})

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

func TestRefreshConfig(t *testing.T) {
	tests := []struct {
		name               string
		callbackOrigin     func(tpl integration.Config) string
		oldValue           string
		newValue           string
		expectedFinalValue string
	}{
		{
			name:               "secret rotation with active config origin",
			callbackOrigin:     func(tpl integration.Config) string { return tpl.Digest() },
			oldValue:           "bar_resolved",
			newValue:           "new_resolved_value",
			expectedFinalValue: "foo: new_resolved_value",
		},
		{
			name:               "callback with non-active config origin",
			callbackOrigin:     func(_ integration.Config) string { return "non-existent-origin" },
			oldValue:           "bar_resolved",
			newValue:           "new_resolved_value",
			expectedFinalValue: "foo: bar_resolved",
		},
		{
			name:               "initial resolution with empty old value",
			callbackOrigin:     func(tpl integration.Config) string { return tpl.Digest() },
			oldValue:           "",
			newValue:           "new_resolved_value",
			expectedFinalValue: "foo: bar_resolved",
		},
		{
			name:               "callback with oldValue as unresolved secret",
			callbackOrigin:     func(tpl integration.Config) string { return tpl.Digest() },
			oldValue:           "ENC[foo]",
			newValue:           "",
			expectedFinalValue: "foo: bar_resolved",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := createDeps(t)
			msch := scheduler.NewControllerAndStart()
			sch := &MockScheduler{scheduled: make(map[string]integration.Config)}
			msch.Register("mock", sch, false)

			tpl := integration.Config{
				Name:          "redisdb",
				ADIdentifiers: []string{"redis"},
				Instances:     []integration.Data{integration.Data("foo: ENC[bar]")},
			}
			mockResolver := MockSecretResolver{t: t, scenarios: []mockSecretScenario{
				{
					expectedData:   []byte{},
					expectedOrigin: tpl.Digest(),
					returnedData:   []byte{},
					returnedError:  nil,
				},
				{
					expectedData:   []byte("foo: ENC[bar]\n"),
					expectedOrigin: tpl.Digest(),
					returnedData:   []byte("foo: bar_resolved"),
					returnedError:  nil,
				},
			}}

			ac := getAutoConfig(msch, &mockResolver, deps.WMeta, deps.TaggerComp, deps.LogsComp, deps.Telemetry, deps.FilterComp)
			ac.processNewService(&dummyService{ID: "abcd", ADIdentifiers: []string{"redis"}})

			changes := ac.processNewConfig(tpl)
			ac.applyChanges(changes)

			resolved := integration.Config{
				Name:          "redisdb",
				ADIdentifiers: []string{"redis"},
				InitConfig:    integration.Data{},
				Instances:     []integration.Data{integration.Data("foo: bar_resolved")},
				MetricConfig:  integration.Data{},
				LogsConfig:    integration.Data{},
				ServiceID:     "abcd",
			}

			require.Eventually(t, func() bool {
				return sch.scheduledSize() == 1 && sch.isScheduled(resolved.Digest())
			}, 5*time.Second, 10*time.Millisecond)

			// rotate secret
			mockResolver.scenarios[1] = mockSecretScenario{
				expectedData:   []byte("foo: ENC[bar]\n"),
				expectedOrigin: tpl.Digest(),
				returnedData:   []byte("foo: " + tt.newValue),
				returnedError:  nil,
			}

			// send subscribers 'secret refreshed' notifications which should
			// queue up autoconfig.refreshConfig()
			mockResolver.triggerCallback(
				"check",
				tt.callbackOrigin(tpl),
				[]string{},
				tt.oldValue,
				tt.newValue,
			)

			resolved = integration.Config{
				Name:          "redisdb",
				ADIdentifiers: []string{"redis"},
				InitConfig:    integration.Data{},
				Instances:     []integration.Data{integration.Data(tt.expectedFinalValue)},
				MetricConfig:  integration.Data{},
				LogsConfig:    integration.Data{},
				ServiceID:     "abcd",
			}

			// newly resolved configuration is eventually scheduled (or remains unchanged if shouldRefresh is false).
			require.Eventually(t, func() bool {
				return sch.scheduledSize() == 1 && sch.isScheduled(resolved.Digest())
			}, 5*time.Second, 10*time.Millisecond)
		})
	}
}

func TestProcessClusterCheckConfigWithSecrets(t *testing.T) {
	deps := createDeps(t)
	configName := "testConfig"

	tpl := integration.Config{
		Provider:     names.ClusterChecks,
		Name:         configName,
		InitConfig:   integration.Data{},
		Instances:    []integration.Data{integration.Data("foo: ENC[bar]")},
		MetricConfig: integration.Data{},
		LogsConfig:   integration.Data{},
	}
	mockResolver := MockSecretResolver{t: t, scenarios: []mockSecretScenario{
		{
			expectedData:   []byte("foo: ENC[bar]"),
			expectedOrigin: tpl.Digest(),
			returnedData:   []byte("foo: barDecoded"),
			returnedError:  nil,
		},
		{
			expectedData:   []byte{},
			expectedOrigin: tpl.Digest(),
			returnedData:   []byte{},
			returnedError:  nil,
		},
	}}
	ac := getAutoConfig(scheduler.NewControllerAndStart(), &mockResolver, deps.WMeta, deps.TaggerComp, deps.LogsComp, deps.Telemetry, deps.FilterComp)
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

	mockResolver := MockSecretResolver{t: t, scenarios: nil}
	ac := getAutoConfig(scheduler.NewControllerAndStart(), &mockResolver, deps.WMeta, deps.TaggerComp, deps.LogsComp, deps.Telemetry, deps.FilterComp)

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
			name:           "With nil Request",
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
			assert.Equal(t, tc.expectedResult, string(result.Configs[0].Config.Instances[0]))

			// Check also that the unresolved configs are returned
			var unresolved []integration.Config
			for _, config := range result.Unresolved {
				unresolved = append(unresolved, config)
			}
			require.Len(t, unresolved, 1)
			assert.Equal(t, tc.expectedResult, string(unresolved[0].Instances[0]))
		})
	}
}

type Deps struct {
	fx.In
	WMeta      option.Option[workloadmeta.Component]
	TaggerComp tagger.Component
	LogsComp   log.Component
	FilterComp workloadfilter.Component
	Telemetry  telemetry.Component
}

func createDeps(t *testing.T) Deps {
	return fxutil.Test[Deps](t,
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
		workloadfilterfxmock.MockModule(),
		fx.Provide(func() tagger.Component { return taggerfxmock.SetupFakeTagger(t) }),
	)
}

func TestAutoConfigTestSuite(t *testing.T) {
	suite.Run(t, new(AutoConfigTestSuite))
}
