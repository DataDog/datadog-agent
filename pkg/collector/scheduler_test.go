// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collector

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	collectorcomp "github.com/DataDog/datadog-agent/comp/collector/collector/def"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	nooptagger "github.com/DataDog/datadog-agent/comp/core/tagger/impl-noop"
	workloadfilterfxmock "github.com/DataDog/datadog-agent/comp/core/workloadfilter/fx-mock"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	collectoraggregator "github.com/DataDog/datadog-agent/pkg/collector/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/infratags"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

type MockCheck struct {
	core.CheckBase
	Name       string
	LoaderName string
	CheckID    checkid.ID
}

func (m MockCheck) Run() error {
	// not used in test
	panic("implement me")
}

func (m MockCheck) Loader() string {
	return m.LoaderName
}

func (m MockCheck) String() string {
	return m.Name
}

func (m MockCheck) ID() checkid.ID {
	return m.CheckID
}

type MockCoreLoader struct{}

func (l *MockCoreLoader) Name() string {
	return "core"
}

// Load loads a check
func (l *MockCoreLoader) Load(_ sender.SenderManager, config integration.Config, _ integration.Data, _ int) (check.Check, error) {
	mockCheck := MockCheck{Name: config.Name, LoaderName: l.Name()}
	return &mockCheck, nil
}

type MockPythonLoader struct{}

func (l *MockPythonLoader) Name() string {
	return "python"
}

// Load loads a check
func (l *MockPythonLoader) Load(_ sender.SenderManager, config integration.Config, _ integration.Data, _ int) (check.Check, error) {
	mockCheck := MockCheck{Name: config.Name, LoaderName: l.Name()}
	return &mockCheck, nil
}

func TestAddLoader(t *testing.T) {
	s := CheckScheduler{}
	assert.Len(t, s.loaders, 0)
	s.addLoader(&MockCoreLoader{})
	s.addLoader(&MockCoreLoader{}) // noop
	assert.Len(t, s.loaders, 1)
}

func TestGetChecksFromConfigs(t *testing.T) {
	s := CheckScheduler{}
	assert.Len(t, s.loaders, 0)
	s.addLoader(&MockCoreLoader{})
	s.addLoader(&MockPythonLoader{})

	// test instance level loader selection
	conf1 := integration.Config{
		Name: "check_a",
		Instances: []integration.Data{
			integration.Data("{\"loader\": \"python\"}"),
			integration.Data("{\"loader\": \"core\"}"),
			integration.Data("{\"loader\": \"wrong\"}"),
			integration.Data("{}"), // default to init config loader
		},
		InitConfig: integration.Data("{\"loader\": \"core\"}"),
	}
	// test init config level loader selection
	conf2 := integration.Config{
		Name:       "check_b",
		Instances:  []integration.Data{integration.Data("{\"value\": 1}")},
		InitConfig: integration.Data("{\"loader\": \"python\"}"),
	}
	// test that wrong loader will be skipped
	conf3 := integration.Config{
		Name:       "check_wrong",
		Instances:  []integration.Data{integration.Data("{\"value\": 1}")},
		InitConfig: integration.Data("{\"loader\": \"wrong_loader\"}"),
	}
	// test that first loader is selected when no loader is selected
	// this is the current behaviour
	conf4 := integration.Config{
		Name:       "check_c",
		Instances:  []integration.Data{integration.Data("{}")},
		InitConfig: integration.Data("{}"),
	}

	checks := s.GetChecksFromConfigs([]integration.Config{conf1, conf2, conf3, conf4}, false)

	assert.Len(t, s.loaders, 2)

	var actualChecks []string

	for _, c := range checks {
		actualChecks = append(actualChecks, c.String())
	}
	assert.Equal(t, []string{
		"check_a",
		"check_a",
		"check_a",
		"check_b",
		"check_c",
	}, actualChecks)
}

func TestGetChecksFromConfigsLoadsSelectedShadowCheckWithSenderManagerOverride(t *testing.T) {
	core.WithTestCatalog(t)
	cfg := configmock.New(t)
	cfg.SetInTest("metric_lookback.enabled", true)
	cfg.SetInTest("metric_lookback.enabled_checks", []string{"cpu"})
	cfg.SetInTest("infrastructure_mode", "cloud_cost_only")

	normalSenderManager := &recordingSchedulerSenderManager{name: "normal"}
	shadowSenderManager := &recordingSchedulerSenderManager{name: "shadow"}
	// Production initializes the aggregator check context before checks are loaded.
	// This lets the scheduler test verify successful rtloader callback registration
	// without leaking the global check context into later tests.
	releaseCheckContext := collectoraggregator.ScopeInitCheckContext(normalSenderManager, option.None[integrations.Component](), nooptagger.NewComponent(), workloadfilterfxmock.SetupMockFilter(t))
	t.Cleanup(releaseCheckContext)

	sourceID := checkid.ID("cpu:loaded-source-id")
	var calls []schedulerLoadCall
	var modes []core.LoadMode
	registerRecordingCoreCheck("cpu", sourceID, false, &calls, &modes)
	loader, err := core.NewGoCheckLoader()
	require.NoError(t, err)
	s := CheckScheduler{
		configToChecks: make(map[string][]checkid.ID),
		senderManager:  normalSenderManager,
		infraTagger:    infratags.NewTagger(cfg),
	}
	s.addLoader(loader)
	s.SetMetricLookbackShadowSenderManager(shadowSenderManager)

	config := integration.Config{
		Name:       "cpu",
		Instances:  []integration.Data{integration.Data("name: first\n")},
		InitConfig: integration.Data("{}"),
	}

	checks := s.GetChecksFromConfigs([]integration.Config{config}, true)

	require.Len(t, checks, 2)
	normalCheck := checks[0]
	shadowCheck := checks[1]
	assert.False(t, check.IsShadow(normalCheck))
	require.True(t, check.IsShadow(shadowCheck))

	assert.Equal(t, sourceID, normalCheck.ID())
	assert.Equal(t, check.ShadowID(sourceID), shadowCheck.ID())
	assert.Equal(t, time.Second, shadowCheck.Interval())
	assert.Equal(t, []core.LoadMode{core.NormalLoadMode, core.ShadowLoadMode}, modes)

	shadowSenderOverride, ok := check.SenderManagerOverride(shadowCheck)
	require.True(t, ok)
	shadowSenderOverrideAdapter, ok := shadowSenderOverride.(*shadowCheckSenderManager)
	require.True(t, ok)
	assert.Same(t, shadowSenderManager, shadowSenderOverrideAdapter.SenderManager)

	assert.Equal(t, []checkid.ID{sourceID, check.ShadowID(sourceID)}, s.configToChecks[config.Digest()])
	require.Len(t, calls, 2)
	assert.Same(t, normalSenderManager, calls[0].senderManager)
	shadowLoadSenderManager, ok := calls[1].senderManager.(*shadowCheckSenderManager)
	require.True(t, ok)
	assert.Same(t, shadowSenderManager, shadowLoadSenderManager.SenderManager)
	assert.NotContains(t, string(calls[0].instance), "_datadog")
	assert.Contains(t, string(calls[1].instance), "_datadog")
	assert.Contains(t, string(calls[1].instance), "execution_mode")
	assert.NotEqual(t, check.ShadowID(sourceID), calls[1].checkID)
	assert.Equal(t, []checkid.ID{sourceID, sourceID}, normalSenderManager.requestedIDs)
	assert.Equal(t, []checkid.ID{sourceID}, normalSenderManager.infraTaggedIDs)
	assert.Equal(t, []checkid.ID{check.ShadowID(sourceID), check.ShadowID(sourceID)}, shadowSenderManager.requestedIDs)
	assert.Equal(t, []checkid.ID{check.ShadowID(sourceID)}, shadowSenderManager.infraTaggedIDs)
}

func TestGetChecksFromConfigsDoesNotLoadShadowChecksWhenCacheIsNotPopulated(t *testing.T) {
	core.WithTestCatalog(t)
	cfg := configmock.New(t)
	cfg.SetInTest("metric_lookback.enabled", true)
	cfg.SetInTest("metric_lookback.enabled_checks", []string{"cpu"})

	normalSenderManager := &recordingSchedulerSenderManager{name: "normal"}
	shadowSenderManager := &recordingSchedulerSenderManager{name: "shadow"}
	var calls []schedulerLoadCall
	var modes []core.LoadMode
	registerRecordingCoreCheck("cpu", "", false, &calls, &modes)
	loader, err := core.NewGoCheckLoader()
	require.NoError(t, err)
	s := CheckScheduler{
		configToChecks:      make(map[string][]checkid.ID),
		senderManager:       normalSenderManager,
		shadowSenderManager: shadowSenderManager,
	}
	s.addLoader(loader)

	config := integration.Config{
		Name:       "cpu",
		Instances:  []integration.Data{integration.Data("name: first\n")},
		InitConfig: integration.Data("{}"),
	}

	checks := s.GetChecksFromConfigs([]integration.Config{config}, false)

	require.Len(t, checks, 1)
	assert.False(t, check.IsShadow(checks[0]))
	assert.Empty(t, s.configToChecks)
	assert.Equal(t, []core.LoadMode{core.NormalLoadMode}, modes)
	require.Len(t, calls, 1)
	assert.Same(t, normalSenderManager, calls[0].senderManager)
	assert.Empty(t, shadowSenderManager.requestedIDs)
}

func TestGetChecksFromConfigsKeepsNormalCheckWhenShadowSenderManagerMissing(t *testing.T) {
	core.WithTestCatalog(t)
	cfg := configmock.New(t)
	cfg.SetInTest("metric_lookback.enabled", true)
	cfg.SetInTest("metric_lookback.enabled_checks", []string{"cpu"})

	normalSenderManager := &recordingSchedulerSenderManager{name: "normal"}
	var calls []schedulerLoadCall
	var modes []core.LoadMode
	registerRecordingCoreCheck("cpu", "", false, &calls, &modes)
	loader, err := core.NewGoCheckLoader()
	require.NoError(t, err)
	s := CheckScheduler{
		configToChecks: make(map[string][]checkid.ID),
		senderManager:  normalSenderManager,
	}
	s.addLoader(loader)

	config := integration.Config{
		Name:       "cpu",
		Instances:  []integration.Data{integration.Data("name: first\n")},
		InitConfig: integration.Data("{}"),
	}

	checks := s.GetChecksFromConfigs([]integration.Config{config}, true)

	require.Len(t, checks, 1)
	assert.False(t, check.IsShadow(checks[0]))
	assert.Equal(t, []checkid.ID{checks[0].ID()}, s.configToChecks[config.Digest()])
	assert.Equal(t, []core.LoadMode{core.NormalLoadMode}, modes)
	require.Len(t, calls, 1)
	assert.Same(t, normalSenderManager, calls[0].senderManager)
	assert.Nil(t, s.shadowSenderManager)
}

func TestGetChecksFromConfigsKeepsNormalCheckWhenShadowLoadFails(t *testing.T) {
	core.WithTestCatalog(t)
	cfg := configmock.New(t)
	cfg.SetInTest("metric_lookback.enabled", true)
	cfg.SetInTest("metric_lookback.enabled_checks", []string{"cpu"})

	normalSenderManager := &recordingSchedulerSenderManager{name: "normal"}
	shadowSenderManager := &recordingSchedulerSenderManager{name: "shadow"}
	var calls []schedulerLoadCall
	var modes []core.LoadMode
	registerRecordingCoreCheck("cpu", "", true, &calls, &modes)
	loader, err := core.NewGoCheckLoader()
	require.NoError(t, err)
	s := CheckScheduler{
		configToChecks:      make(map[string][]checkid.ID),
		senderManager:       normalSenderManager,
		shadowSenderManager: shadowSenderManager,
	}
	s.addLoader(loader)

	config := integration.Config{
		Name:       "cpu",
		Instances:  []integration.Data{integration.Data("name: first\n")},
		InitConfig: integration.Data("{}"),
	}

	checks := s.GetChecksFromConfigs([]integration.Config{config}, true)

	require.Len(t, checks, 1)
	assert.False(t, check.IsShadow(checks[0]))
	assert.Equal(t, []checkid.ID{checks[0].ID()}, s.configToChecks[config.Digest()])
	assert.Equal(t, []core.LoadMode{core.NormalLoadMode, core.ShadowLoadMode}, modes)
	assert.Len(t, calls, 2)
	assert.Equal(t, []checkid.ID{check.ShadowID(checks[0].ID())}, shadowSenderManager.destroyedIDs)
}

func TestGetChecksFromConfigsSkipsShadowCheckForUnsupportedLoader(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("metric_lookback.enabled", true)

	normalSenderManager := &recordingSchedulerSenderManager{name: "normal"}
	shadowSenderManager := &recordingSchedulerSenderManager{name: "shadow"}
	loader := &recordingSchedulerLoader{name: "sharedlibrary"}
	s := CheckScheduler{
		configToChecks:      make(map[string][]checkid.ID),
		senderManager:       normalSenderManager,
		shadowSenderManager: shadowSenderManager,
	}
	s.addLoader(loader)

	config := integration.Config{
		Name:       "custom_native",
		Instances:  []integration.Data{integration.Data("name: first\n")},
		InitConfig: integration.Data("{}"),
	}

	checks := s.GetChecksFromConfigs([]integration.Config{config}, true)

	require.Len(t, checks, 1)
	assert.False(t, check.IsShadow(checks[0]))
	assert.Equal(t, []checkid.ID{checks[0].ID()}, s.configToChecks[config.Digest()])
	require.Len(t, loader.calls, 1)
	assert.Same(t, normalSenderManager, loader.calls[0].senderManager)
	assert.Empty(t, shadowSenderManager.requestedIDs)
	assert.Empty(t, shadowSenderManager.destroyedIDs)
}

func TestShadowLoaderForPythonReusesLoadedLoader(t *testing.T) {
	loader := &recordingSchedulerLoader{name: "python"}
	s := CheckScheduler{}

	shadowLoader, ok := s.shadowLoaderFor(loader)

	require.True(t, ok)
	assert.Same(t, loader, shadowLoader)
}

func TestShadowLoaderForCoreUsesShadowLoadMode(t *testing.T) {
	loader, err := core.NewGoCheckLoader()
	require.NoError(t, err)
	s := CheckScheduler{}

	shadowLoader, ok := s.shadowLoaderFor(loader)

	require.True(t, ok)
	shadowCoreLoader, ok := shadowLoader.(*core.GoCheckLoader)
	require.True(t, ok)
	assert.Equal(t, core.ShadowLoadMode, shadowCoreLoader.LoadMode())
}

func TestShadowLoaderForCoreReusesShadowLoader(t *testing.T) {
	loader, err := core.NewGoCheckLoader()
	require.NoError(t, err)
	s := CheckScheduler{}

	firstShadowLoader, ok := s.shadowLoaderFor(loader)
	require.True(t, ok)
	secondShadowLoader, ok := s.shadowLoaderFor(loader)

	require.True(t, ok)
	assert.Same(t, firstShadowLoader, secondShadowLoader)
}

// MockCollector is a mock implementation of collectorcomp.Component for testing
type MockCollector struct {
	RunCheckCalls []check.Check // Track which checks were run
	RunCheckError error         // Error to return from RunCheck
}

type schedulerLoadCall struct {
	senderManager sender.SenderManager
	config        integration.Config
	instance      integration.Data
	instanceIndex int
	checkID       checkid.ID
}

type recordingSchedulerLoader struct {
	name           string
	normalCheckID  checkid.ID
	failShadowLoad bool
	calls          []schedulerLoadCall
}

func (l *recordingSchedulerLoader) Name() string {
	return l.name
}

func (l *recordingSchedulerLoader) Load(senderManager sender.SenderManager, config integration.Config, instance integration.Data, instanceIndex int) (check.Check, error) {
	checkID := checkid.BuildID(config.Name, config.FastDigest(), instance, config.InitConfig)
	if l.normalCheckID != "" && !bytes.Contains(instance, []byte("_datadog")) {
		checkID = l.normalCheckID
	}
	l.calls = append(l.calls, schedulerLoadCall{
		senderManager: senderManager,
		config:        config,
		instance:      append(integration.Data(nil), instance...),
		instanceIndex: instanceIndex,
		checkID:       checkID,
	})
	if l.failShadowLoad && bytes.Contains(instance, []byte("_datadog")) {
		return nil, errors.New("shadow load failed")
	}
	if _, err := senderManager.GetSender(checkID); err != nil {
		return nil, err
	}
	return &MockCheck{
		Name:       config.Name,
		LoaderName: l.Name(),
		CheckID:    checkID,
	}, nil
}

type recordingCoreCheck struct {
	MockCheck
	mode           core.LoadMode
	normalCheckID  checkid.ID
	failShadowLoad bool
	calls          *[]schedulerLoadCall
	modes          *[]core.LoadMode
}

func registerRecordingCoreCheck(name string, normalCheckID checkid.ID, failShadowLoad bool, calls *[]schedulerLoadCall, modes *[]core.LoadMode) {
	core.RegisterContextualCheck(name, option.New(func(ctx core.ConstructionContext) check.Check {
		return &recordingCoreCheck{
			MockCheck: MockCheck{
				Name:       name,
				LoaderName: core.GoCheckLoaderName,
			},
			mode:           ctx.Mode,
			normalCheckID:  normalCheckID,
			failShadowLoad: failShadowLoad,
			calls:          calls,
			modes:          modes,
		}
	}))
}

func (c *recordingCoreCheck) Configure(senderManager sender.SenderManager, digest uint64, instance integration.Data, initConfig integration.Data, _ string, _ string) error {
	checkID := checkid.BuildID(c.Name, digest, instance, initConfig)
	if c.normalCheckID != "" && c.mode == core.NormalLoadMode {
		checkID = c.normalCheckID
	}
	c.CheckID = checkID
	*c.calls = append(*c.calls, schedulerLoadCall{
		senderManager: senderManager,
		config:        integration.Config{Name: c.Name, InitConfig: initConfig},
		instance:      append(integration.Data(nil), instance...),
		checkID:       checkID,
	})
	*c.modes = append(*c.modes, c.mode)
	if c.failShadowLoad && c.mode == core.ShadowLoadMode {
		return errors.New("shadow load failed")
	}
	_, err := senderManager.GetSender(checkID)
	return err
}

type recordingSchedulerSenderManager struct {
	sender.SenderManager
	name           string
	requestedIDs   []checkid.ID
	destroyedIDs   []checkid.ID
	infraTaggedIDs []checkid.ID
}

func (m *recordingSchedulerSenderManager) GetSender(id checkid.ID) (sender.Sender, error) {
	m.requestedIDs = append(m.requestedIDs, id)
	return &recordingSchedulerSender{id: id, manager: m}, nil
}

func (m *recordingSchedulerSenderManager) DestroySender(id checkid.ID) {
	m.destroyedIDs = append(m.destroyedIDs, id)
}

type recordingSchedulerSender struct {
	sender.Sender
	id      checkid.ID
	manager *recordingSchedulerSenderManager
}

func (s *recordingSchedulerSender) SetInfraTagger(*infratags.Tagger) {
	s.manager.infraTaggedIDs = append(s.manager.infraTaggedIDs, s.id)
}

func (m *MockCollector) RunCheck(c check.Check) (checkid.ID, error) {
	m.RunCheckCalls = append(m.RunCheckCalls, c)
	if m.RunCheckError != nil {
		return "", m.RunCheckError
	}
	return c.ID(), nil
}

func (m *MockCollector) StopCheck(_ checkid.ID) error {
	return nil
}

func (m *MockCollector) ReloadAllCheckInstances(_ string, _ []check.Check) ([]checkid.ID, error) {
	return nil, nil
}

func (m *MockCollector) GetChecks() []check.Check {
	return nil
}

func (m *MockCollector) MapOverChecks(cb func([]check.Info)) {
	cb(nil)
}

func (m *MockCollector) AddEventReceiver(_ collectorcomp.EventReceiver) {
}

func TestSchedule_AllChecksAllowed(t *testing.T) {
	// Test that when not in basic mode, all checks are scheduled
	mockCollector := &MockCollector{}
	s := &CheckScheduler{
		collector:      option.New[collectorcomp.Component](mockCollector),
		configToChecks: make(map[string][]checkid.ID),
	}
	s.addLoader(&MockCoreLoader{})

	configs := []integration.Config{
		{
			Name:       "cpu",
			Instances:  []integration.Data{integration.Data("{}")},
			InitConfig: integration.Data("{}"),
		},
		{
			Name:       "disk",
			Instances:  []integration.Data{integration.Data("{}")},
			InitConfig: integration.Data("{}"),
		},
		{
			Name:       "custom_check", // Test custom_.* pattern
			Instances:  []integration.Data{integration.Data("{}")},
			InitConfig: integration.Data("{}"),
		},
	}

	s.Schedule(configs)

	// All checks should be run when not in basic mode
	assert.Len(t, mockCollector.RunCheckCalls, len(configs))
	for i, c := range configs {
		assert.Equal(t, c.Name, mockCollector.RunCheckCalls[i].(*MockCheck).Name)
	}
}
