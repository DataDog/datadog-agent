// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package autodiscoveryimpl

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"github.com/mohae/deepcopy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/listeners"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues/ad-misconfiguration"
	healthplatformmock "github.com/DataDog/datadog-agent/comp/healthplatform/store/mock"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/util/testutil"
)

func noopConfigPublisher(integration.ConfigChanges, map[checkid.ID]checkid.ID) {}

func TestConfigManagerPublishesBeforeUnlock(t *testing.T) {
	cm := newReconcilingConfigManager(&MockSecretResolver{}, nil, nil, nil, nil, noopConfigPublisher).(*reconcilingConfigManager)
	var published []integration.ConfigChanges
	cm.publish = func(changes integration.ConfigChanges, _ map[checkid.ID]checkid.ID) {
		// A concurrent mutation cannot pass the manager lock while this batch
		// is being published. Unlock on failure so the test can report it.
		if cm.m.TryLock() {
			cm.m.Unlock()
			t.Error("publication must hold the manager's mutation lock")
		}
		published = append(published, changes)
	}

	tpl := integration.Config{Name: "vault", ADIdentifiers: []string{"vault"}, LogsConfig: integration.Data(`[{"source":"vault"}]`)}
	svc := &dummyService{ID: "docker://vault", ADIdentifiers: []string{"vault"}}
	cm.processNewConfig(tpl)
	cm.processNewService(svc)
	cm.processDelService(svc)
	cm.processDelConfigs([]integration.Config{tpl})
	require.Len(t, published, 4)
	require.True(t, published[0].IsEmpty())
	require.Len(t, published[1].Schedule, 1)
	require.Len(t, published[2].Unschedule, 1)
	require.Equal(t, published[1].Schedule, published[2].Unschedule)
	require.True(t, published[3].IsEmpty())
}

// assertConfigsMatch verifies that the given slice of changes has exactly
// one match to each of the given functions, regardless of order.
func assertConfigsMatch(t *testing.T, configs []integration.Config, matches ...func(integration.Config) bool) {
	matchCount := make([]int, len(matches))

	for _, config := range configs {
		configMatched := false
		for i, f := range matches {
			if f(config) {
				matchCount[i]++
				configMatched = true
			}
		}
		if !configMatched {
			t.Errorf("Config %#v did not match any of matches", config)
		}
	}

	for i, count := range matchCount {
		if count != 1 {
			t.Errorf("matches[%d] matched %d times", i, count)
		}
	}
}

// assertLoadedConfigsMatch asserts that the set of loaded configs on the given
// configManager matches the given functions.
func assertLoadedConfigsMatch(t *testing.T, cm configManager, matches ...func(integration.Config) bool) {
	var configs []integration.Config
	cm.mapOverLoadedConfigs(func(loaded map[string]integration.Config) {
		for _, cfg := range loaded {
			configs = append(configs, cfg)
		}
	})

	assertConfigsMatch(t, configs, matches...)
}

// matchAll matches when all of the given functions match
func matchAll(matches ...func(integration.Config) bool) func(integration.Config) bool {
	return func(config integration.Config) bool {
		for _, f := range matches {
			if !f(config) {
				return false
			}
		}
		return true
	}
}

// matchName matches config.Name
func matchName(name string) func(integration.Config) bool {
	return func(config integration.Config) bool {
		return config.Name == name
	}
}

func matchDigest(digest string) func(integration.Config) bool {
	return func(config integration.Config) bool {
		return config.Digest() == digest
	}
}

// matchLogsConfig matches config.LogsConfig (for verifying templates are applied)
func matchLogsConfig(logsConfig string) func(integration.Config) bool {
	return func(config integration.Config) bool {
		return string(config.LogsConfig) == logsConfig
	}
}

// matchLogsConfig matches config.LogsConfig (for verifying templates are applied)
func matchSvc(serviceID string) func(integration.Config) bool {
	return func(config integration.Config) bool {
		return config.ServiceID == serviceID
	}
}

var (
	nonTemplateConfig             = integration.Config{Name: "non-template"}
	nonTemplateConfigWithSecrets  = integration.Config{Name: "non-template-with-secrets", Instances: []integration.Data{integration.Data("foo: ENC[bar]")}}
	clusterCheckConfigWithSecrets = integration.Config{
		Provider:  names.ClusterChecks,
		Name:      "non-template-with-secrets-cluster-check",
		Instances: []integration.Data{integration.Data("foo: ENC[bar]")},
	}
	templateConfig = integration.Config{Name: "template", LogsConfig: []byte("source: %%host%%"), ADIdentifiers: []string{"my-service"}}
	myService      = &dummyService{ID: "my-service", ADIdentifiers: []string{"my-service"}, Hosts: map[string]string{"main": "myhost"}}
)

type ConfigManagerSuite struct {
	suite.Suite
	factory    func(func(integration.ConfigChanges, map[checkid.ID]checkid.ID)) configManager
	changes    integration.ConfigChanges
	changedIDs map[checkid.ID]checkid.ID
	cm         configManager
}

func (suite *ConfigManagerSuite) SetupTest() {
	suite.cm = suite.factory(suite.captureChanges)
}

func (suite *ConfigManagerSuite) captureChanges(changes integration.ConfigChanges, ids map[checkid.ID]checkid.ID) {
	suite.changes = changes
	suite.changedIDs = ids
}

// A new, non-template config is scheduled immediately and unscheduled when
// deleted
func (suite *ConfigManagerSuite) TestNewNonTemplateScheduled() {
	suite.cm.processNewConfig(nonTemplateConfig)
	assertConfigsMatch(suite.T(), suite.changes.Schedule, matchName("non-template"))
	assertConfigsMatch(suite.T(), suite.changes.Unschedule)

	suite.cm.processDelConfigs([]integration.Config{nonTemplateConfig})
	assertConfigsMatch(suite.T(), suite.changes.Schedule)
	assertConfigsMatch(suite.T(), suite.changes.Unschedule, matchName("non-template"))
}

// A new, non-template config with secrets is scheduled immediately and unscheduled when
// deleted
func (suite *ConfigManagerSuite) TestNewNonTemplateWithSecretsScheduled() {
	inputNewConfig := deepcopy.Copy(nonTemplateConfigWithSecrets).(integration.Config)
	mockResolver := MockSecretResolver{t: suite.T(), scenarios: []mockSecretScenario{
		{
			expectedData:   []byte("foo: ENC[bar]"),
			expectedOrigin: inputNewConfig.Digest(),
			returnedData:   []byte("foo: barDecoded"),
			returnedError:  nil,
		},
		{
			expectedData:   []byte{},
			expectedOrigin: inputNewConfig.Digest(),
			returnedData:   []byte{},
			returnedError:  nil,
		},
	}}
	// assign mock secretResolver to the configManager
	cm := suite.cm.(*reconcilingConfigManager)
	cm.secretResolver = &mockResolver

	suite.cm.processNewConfig(inputNewConfig)

	assert.Empty(suite.T(), suite.changedIDs) // Only published if the config provider is cluster-checks.

	assertConfigsMatch(suite.T(), suite.changes.Schedule, matchName(nonTemplateConfigWithSecrets.Name))
	assertConfigsMatch(suite.T(), suite.changes.Unschedule)
	// Verify content is actually decoded

	require.True(suite.T(), strings.Contains(string(suite.changes.Schedule[0].Instances[0]), "barDecoded"))
	newConfigDigest := suite.changes.Schedule[0].Digest()

	inputDelConfig := deepcopy.Copy(nonTemplateConfigWithSecrets).(integration.Config)
	suite.cm.processDelConfigs([]integration.Config{inputDelConfig})
	assertConfigsMatch(suite.T(), suite.changes.Schedule)
	assertConfigsMatch(suite.T(), suite.changes.Unschedule, matchName(nonTemplateConfigWithSecrets.Name))
	assertConfigsMatch(suite.T(), suite.changes.Unschedule, matchDigest(newConfigDigest))
	require.True(suite.T(), strings.Contains(string(suite.changes.Unschedule[0].Instances[0]), "barDecoded"))
}

func (suite *ConfigManagerSuite) TestNewClusterCheckWithSecretsScheduled() {
	inputNewConfig := deepcopy.Copy(clusterCheckConfigWithSecrets).(integration.Config)
	mockResolver := &MockSecretResolver{t: suite.T(), scenarios: []mockSecretScenario{
		{
			expectedData:   []byte("foo: ENC[bar]"),
			expectedOrigin: inputNewConfig.Digest(),
			returnedData:   []byte("foo: barDecoded"),
			returnedError:  nil,
		},
		{
			expectedData:   []byte{},
			expectedOrigin: inputNewConfig.Digest(),
			returnedData:   []byte{},
			returnedError:  nil,
		},
	}}
	// assign mock secretResolver to the configManager
	cm := suite.cm.(*reconcilingConfigManager)
	cm.secretResolver = mockResolver

	suite.cm.processNewConfig(inputNewConfig)

	// Check that the published IDs contain the correct mapping of IDs
	originalCheckID := checkid.BuildID(
		clusterCheckConfigWithSecrets.Name,
		clusterCheckConfigWithSecrets.FastDigest(),
		clusterCheckConfigWithSecrets.Instances[0],
		clusterCheckConfigWithSecrets.InitConfig,
	)
	newCheckID := checkid.BuildID(
		suite.changes.Schedule[0].Name,
		suite.changes.Schedule[0].FastDigest(),
		suite.changes.Schedule[0].Instances[0],
		suite.changes.Schedule[0].InitConfig,
	)
	assert.Equal(suite.T(), map[checkid.ID]checkid.ID{newCheckID: originalCheckID}, suite.changedIDs)

	assertConfigsMatch(suite.T(), suite.changes.Schedule, matchName(clusterCheckConfigWithSecrets.Name))
	assertConfigsMatch(suite.T(), suite.changes.Unschedule)
	// Verify content is actually decoded
	require.True(suite.T(), strings.Contains(string(suite.changes.Schedule[0].Instances[0]), "barDecoded"))
	newConfigDigest := suite.changes.Schedule[0].Digest()

	inputDelConfig := deepcopy.Copy(clusterCheckConfigWithSecrets).(integration.Config)
	suite.cm.processDelConfigs([]integration.Config{inputDelConfig})
	assertConfigsMatch(suite.T(), suite.changes.Schedule)
	assertConfigsMatch(suite.T(), suite.changes.Unschedule, matchName(clusterCheckConfigWithSecrets.Name))
	assertConfigsMatch(suite.T(), suite.changes.Unschedule, matchDigest(newConfigDigest))
	require.True(suite.T(), strings.Contains(string(suite.changes.Unschedule[0].Instances[0]), "barDecoded"))
}

// A new template config is not scheduled when there is no matching service, and
// not unscheduled when removed
func (suite *ConfigManagerSuite) TestNewTemplateNotScheduled() {
	suite.cm.processNewConfig(templateConfig)
	assertConfigsMatch(suite.T(), suite.changes.Schedule)
	assertConfigsMatch(suite.T(), suite.changes.Unschedule)

	suite.cm.processDelConfigs([]integration.Config{templateConfig})
	assertConfigsMatch(suite.T(), suite.changes.Schedule)
	assertConfigsMatch(suite.T(), suite.changes.Unschedule)
}

// A new template config is not scheduled when there is no matching service, but
// is resolved and scheduled when such a service arrives; deleting the config
// unschedules the resolved configs.
func (suite *ConfigManagerSuite) TestNewTemplateBeforeService_ConfigRemovedFirst() {
	suite.cm.processNewConfig(templateConfig)
	assertConfigsMatch(suite.T(), suite.changes.Schedule)
	assertConfigsMatch(suite.T(), suite.changes.Unschedule)

	suite.cm.processNewService(myService)
	assertConfigsMatch(suite.T(), suite.changes.Schedule, matchAll(matchName("template"), matchLogsConfig("source: myhost\n")))
	assertConfigsMatch(suite.T(), suite.changes.Unschedule)

	suite.cm.processDelConfigs([]integration.Config{templateConfig})
	assertConfigsMatch(suite.T(), suite.changes.Schedule)
	assertConfigsMatch(suite.T(), suite.changes.Unschedule, matchAll(matchName("template"), matchLogsConfig("source: myhost\n")))

	suite.cm.processDelService(myService)
	assertConfigsMatch(suite.T(), suite.changes.Schedule)
	assertConfigsMatch(suite.T(), suite.changes.Unschedule)
}

// A new template config is not scheduled when there is no matching service, but
// is resolved and scheduled when such a service arrives; deleting the service
// unschedules the resolved configs.
func (suite *ConfigManagerSuite) TestNewTemplateBeforeService_ServiceRemovedFirst() {
	suite.cm.processNewConfig(templateConfig)
	assertConfigsMatch(suite.T(), suite.changes.Schedule)
	assertConfigsMatch(suite.T(), suite.changes.Unschedule)

	suite.cm.processNewService(myService)
	assertConfigsMatch(suite.T(), suite.changes.Schedule, matchAll(matchName("template"), matchLogsConfig("source: myhost\n")))
	assertConfigsMatch(suite.T(), suite.changes.Unschedule)

	suite.cm.processDelService(myService)
	assertConfigsMatch(suite.T(), suite.changes.Schedule)
	assertConfigsMatch(suite.T(), suite.changes.Unschedule, matchAll(matchName("template"), matchLogsConfig("source: myhost\n")))

	suite.cm.processDelConfigs([]integration.Config{templateConfig})
	assertConfigsMatch(suite.T(), suite.changes.Schedule)
	assertConfigsMatch(suite.T(), suite.changes.Unschedule)
}

// A new service is not scheduled when there is no matching template, but
// is resolved and scheduled when such a template arrives; deleting the template
// unschedules the resolved configs.
func (suite *ConfigManagerSuite) TestNewServiceBeforeTemplate_ConfigRemovedFirst() {
	suite.cm.processNewService(myService)
	assertConfigsMatch(suite.T(), suite.changes.Schedule)
	assertConfigsMatch(suite.T(), suite.changes.Unschedule)

	suite.cm.processNewConfig(templateConfig)
	assertConfigsMatch(suite.T(), suite.changes.Schedule, matchAll(matchName("template"), matchLogsConfig("source: myhost\n")))
	assertConfigsMatch(suite.T(), suite.changes.Unschedule)

	suite.cm.processDelConfigs([]integration.Config{templateConfig})
	assertConfigsMatch(suite.T(), suite.changes.Schedule)
	assertConfigsMatch(suite.T(), suite.changes.Unschedule, matchAll(matchName("template"), matchLogsConfig("source: myhost\n")))

	suite.cm.processDelService(myService)
	assertConfigsMatch(suite.T(), suite.changes.Schedule)
	assertConfigsMatch(suite.T(), suite.changes.Unschedule)
}

// A new service is not scheduled when there is no matching template, but
// is resolved and scheduled when such a template arrives; deleting the service
// unschedules the resolved configs.
func (suite *ConfigManagerSuite) TestNewServiceBeforeTemplate_ServiceRemovedFirst() {
	suite.cm.processNewService(myService)
	assertConfigsMatch(suite.T(), suite.changes.Schedule)
	assertConfigsMatch(suite.T(), suite.changes.Unschedule)

	suite.cm.processNewConfig(templateConfig)
	assertConfigsMatch(suite.T(), suite.changes.Schedule, matchAll(matchName("template"), matchLogsConfig("source: myhost\n")))
	assertConfigsMatch(suite.T(), suite.changes.Unschedule)

	suite.cm.processDelService(myService)
	assertConfigsMatch(suite.T(), suite.changes.Schedule)
	assertConfigsMatch(suite.T(), suite.changes.Unschedule, matchAll(matchName("template"), matchLogsConfig("source: myhost\n")))

	suite.cm.processDelConfigs([]integration.Config{templateConfig})
	assertConfigsMatch(suite.T(), suite.changes.Schedule)
	assertConfigsMatch(suite.T(), suite.changes.Unschedule)
}

// Fuzz the config manager to ensure it doesn't "leak" configs -- that schedule
// and unschedule calls are always properly paired.
func (suite *ConfigManagerSuite) TestFuzz() {
	testutil.Fuzz(suite.T(), func(seed int64) {
		fmt.Printf("==== starting fuzz with random seed %d\n", seed)
		r := rand.New(rand.NewSource(seed))

		scheduled := map[string]struct{}{} // currently-scheduled config digests

		// apply the given changes, checking for double-schedules and double-unschedules
		applyChanges := func(changes integration.ConfigChanges) {
			for _, cfg := range changes.Unschedule {
				digest := cfg.Digest()
				fmt.Printf("unschedule config %s -- Name: %#v, ADIdentifiers: [%s], ServiceID: %#v\n",
					digest, cfg.Name, strings.Join(cfg.ADIdentifiers, ", "), cfg.ServiceID)
				if _, found := scheduled[digest]; !found {
					suite.T().Fatalf("config is not scheduled")
				}
				delete(scheduled, digest)
			}
			for _, cfg := range changes.Schedule {
				digest := cfg.Digest()
				fmt.Printf("schedule config %s -- Name: %#v, ADIdentifiers: [%s], ServiceID: %#v\n",
					digest, cfg.Name, strings.Join(cfg.ADIdentifiers, ", "), cfg.ServiceID)
				if _, found := scheduled[digest]; found {
					suite.T().Fatalf("config is already scheduled")
				}
				scheduled[digest] = struct{}{}
			}
		}

		cm := suite.factory(func(changes integration.ConfigChanges, _ map[checkid.ID]checkid.ID) { applyChanges(changes) })

		// generate a random string with the given prefix, with N possible outcomes
		randStr := func(pfx string, n int, r *rand.Rand) string {
			return fmt.Sprintf("%s%d", pfx, r.Intn(n)+1)
		}

		// return an array of AD identifiers
		randADIDs := func(r *rand.Rand) []string {
			adIdentifiers := make([]string, r.Intn(5)+1)
			for i := range adIdentifiers {
				adIdentifiers[i] = randStr("ad", 50, r)
			}
			return adIdentifiers
		}

		// make a random non-template config
		makeNonTemplateConfig := func(r *rand.Rand) integration.Config {
			return integration.Config{Name: randStr("cfg", 10, r)}
		}

		// make a random template config
		makeTemplateConfig := func(r *rand.Rand) integration.Config {
			return integration.Config{Name: randStr("tpl", 15, r), ADIdentifiers: randADIDs(r)}
		}

		// make a random service
		makeService := func(r *rand.Rand) listeners.Service {
			return &dummyService{ID: randStr("svc", 15, r), ADIdentifiers: randADIDs(r)}
		}

		op := 0
		removeAfterOps := 10
		configs := map[string]integration.Config{}
		services := map[string]listeners.Service{}
		for {
			p := r.Intn(90)
			switch {
			case p < 20 && op < removeAfterOps: // add service
				svc := makeService(r)
				id := svc.GetServiceID()
				adIDs := svc.GetADIdentifiers()
				if _, found := services[id]; !found {
					services[id] = svc
					fmt.Printf("add service %s with AD idents [%s]\n", id, strings.Join(adIDs, ", "))
					cm.processNewService(svc)
				}
			case p < 40 && op < removeAfterOps: // add non-template config
				cfg := makeNonTemplateConfig(r)
				digest := cfg.Digest()
				if _, found := configs[digest]; !found {
					configs[digest] = cfg
					fmt.Printf("add non-template config %s (digest %s)\n", cfg.Name, digest)
					cm.processNewConfig(cfg)
				}
			case p < 60 && op < removeAfterOps: // add template config
				cfg := makeTemplateConfig(r)
				digest := cfg.Digest()
				if _, found := configs[digest]; !found {
					configs[digest] = cfg
					fmt.Printf("add template config %s (digest %s) with AD idents [%s]\n",
						cfg.Name, digest, strings.Join(cfg.ADIdentifiers, ", "))
					cm.processNewConfig(cfg)
				}
			case p < 70 && len(services) > 0: // remove service
				i := rand.Intn(len(services))
				for id, svc := range services {
					if i == 0 {
						delete(services, id)
						adIDs := svc.GetADIdentifiers()
						fmt.Printf("remove service %s with AD idents %s\n", id, strings.Join(adIDs, ", "))
						cm.processDelService(svc)
						break
					}
					i--
				}
			case p < 90 && len(configs) > 0: // remove config
				i := rand.Intn(len(configs))
				for digest, cfg := range configs {
					if i == 0 {
						delete(configs, digest)
						if len(cfg.ADIdentifiers) > 0 {
							fmt.Printf("remove template config %s (digest %s) with AD idents [%s]\n",
								cfg.Name, digest, strings.Join(cfg.ADIdentifiers, ", "))
						} else {
							fmt.Printf("remove non-template config %s (digest %s)\n", cfg.Name, digest)
						}
						cm.processDelConfigs([]integration.Config{cfg})
						break
					}
					i--
				}
			}

			// verify that the loaded configs are correct
			cm.mapOverLoadedConfigs(func(loaded map[string]integration.Config) {
				failed := false

				for digest := range scheduled {
					if _, found := loaded[digest]; !found {
						fmt.Printf("config with digest %s is not scheduled and should be", digest)
						failed = true
					}
				}

				for digest := range loaded {
					if _, found := scheduled[digest]; !found {
						fmt.Printf("config with digest %s is scheduled and should not be", digest)
						failed = true
					}
				}
				if failed {
					suite.T().Fatalf("mapOverLoadedConfigs returned unexpected set of configs")
				}
			})

			op++
			if op > removeAfterOps && len(services) == 0 && len(configs) == 0 {
				break
			}
		}

		require.Empty(suite.T(), scheduled, "configs remain scheduled after everything was removed")
	})
}

type ReconcilingConfigManagerSuite struct {
	ConfigManagerSuite // include all ConfigManager tests, and more..
}

// A service's filtering determines which templates are resolved and scheduled.
func (suite *ReconcilingConfigManagerSuite) TestServiceTemplateFiltering() {
	filterSvc := &dummyService{ID: "filter", ADIdentifiers: []string{"filter"}}
	filterSvc.filterTemplates = func(configs map[string]integration.Config) {
		for digest, config := range configs {
			if !strings.HasSuffix(config.Name, "-keep") {
				delete(configs, digest)
			}
		}
	}

	// adding service with no templates has no effect
	suite.cm.processNewService(myService)
	assertConfigsMatch(suite.T(), suite.changes.Schedule)
	assertConfigsMatch(suite.T(), suite.changes.Unschedule)
	assertLoadedConfigsMatch(suite.T(), suite.cm)

	// adding service with no templates has no effect
	suite.cm.processNewService(filterSvc)
	assertConfigsMatch(suite.T(), suite.changes.Schedule)
	assertConfigsMatch(suite.T(), suite.changes.Unschedule)
	assertLoadedConfigsMatch(suite.T(), suite.cm)

	// adding a template that does not end in -keep only matches my-service
	cfg1 := integration.Config{Name: "cfg1", ADIdentifiers: []string{"my-service", "filter"}}
	suite.cm.processNewConfig(cfg1)
	assertConfigsMatch(suite.T(), suite.changes.Schedule, matchAll(matchName("cfg1"), matchSvc("my-service")))
	assertConfigsMatch(suite.T(), suite.changes.Unschedule)
	assertLoadedConfigsMatch(suite.T(), suite.cm, matchAll(matchName("cfg1"), matchSvc("my-service")))

	// adding a template that ends in -keep matches both services
	cfg2 := integration.Config{Name: "cfg2-keep", ADIdentifiers: []string{"my-service", "filter"}}
	suite.cm.processNewConfig(cfg2)
	assertConfigsMatch(suite.T(), suite.changes.Schedule,
		matchAll(matchName("cfg2-keep"), matchSvc("my-service")),
		matchAll(matchName("cfg2-keep"), matchSvc("filter")),
	)
	assertConfigsMatch(suite.T(), suite.changes.Unschedule)
	assertLoadedConfigsMatch(suite.T(), suite.cm,
		matchAll(matchName("cfg1"), matchSvc("my-service")),
		matchAll(matchName("cfg2-keep"), matchSvc("my-service")),
		matchAll(matchName("cfg2-keep"), matchSvc("filter")),
	)

	// removing a service removes only the scheduled configs
	suite.cm.processDelService(filterSvc)
	assertConfigsMatch(suite.T(), suite.changes.Schedule)
	assertConfigsMatch(suite.T(), suite.changes.Unschedule, matchAll(matchName("cfg2-keep"), matchSvc("filter")))
	assertLoadedConfigsMatch(suite.T(), suite.cm,
		matchAll(matchName("cfg1"), matchSvc("my-service")),
		matchAll(matchName("cfg2-keep"), matchSvc("my-service")),
	)
}

func TestReconcilingConfigManagement(t *testing.T) {
	mockResolver := MockSecretResolver{}
	suite.Run(t, &ReconcilingConfigManagerSuite{
		ConfigManagerSuite{factory: func(publish func(integration.ConfigChanges, map[checkid.ID]checkid.ID)) configManager {
			return newReconcilingConfigManager(&mockResolver, nil, nil, nil, nil, publish)
		}},
	})
}

// dummyServiceWithExtraConfigError is a dummyService that returns an error for GetExtraConfig
type dummyServiceWithExtraConfigError struct {
	dummyService
}

func (s *dummyServiceWithExtraConfigError) GetExtraConfig(key string) (string, error) {
	return "", fmt.Errorf("extra config %q is not supported", key)
}

// A service whose FilterTemplates consults the shared StaticConfigIndex
// (as ProcessService does) has its templates correctly deduped when its
// reconciliation runs after a static config has already been published to
// the index. This covers the common startup ordering: static configs from
// conf.d are loaded before the process listener starts discovering
// services.
//
// TODO: runtime ordering (static arrives after dynamic, or static leaves
// while dynamic is scheduled) is not covered yet — see configmgr.go.
func TestStaticConfigIndexDedupOnReconcile(t *testing.T) {
	mockResolver := MockSecretResolver{}
	idx := listeners.NewStaticConfigIndex()

	published := &configChangeRecorder{}
	cm := newReconcilingConfigManager(&mockResolver, nil, idx, nil, nil, published.record)

	// A service whose FilterTemplates drops any template whose Name has a
	// static config in the shared index — this is the contract ProcessService
	// implements.
	procSvc := &dummyService{
		ID:            "process://1234",
		ADIdentifiers: []string{"proc-redis"},
		Hosts:         map[string]string{"main": "127.0.0.1"},
	}
	procSvc.filterTemplates = func(configs map[string]integration.Config) {
		for digest, cfg := range configs {
			if idx.Has(cfg.Name) {
				delete(configs, digest)
			}
		}
	}

	redisTemplate := integration.Config{
		Name:          "redis",
		LogsConfig:    []byte("source: %%host%%"),
		ADIdentifiers: []string{"proc-redis"},
	}
	staticRedis := integration.Config{
		Name:      "redis",
		Instances: []integration.Data{integration.Data("port: 6379")},
	}

	// Startup ordering: static config arrives first, then the template,
	// then the matching service.
	cm.processNewConfig(staticRedis)
	assertConfigsMatch(t, published.last().Schedule, matchName("redis"))
	assertConfigsMatch(t, published.last().Unschedule)
	assert.True(t, idx.Has("redis"))

	cm.processNewConfig(redisTemplate)
	assertConfigsMatch(t, published.last().Schedule)
	assertConfigsMatch(t, published.last().Unschedule)

	// When the service arrives, reconciliation runs FilterTemplates and
	// drops the redis template because the index reports a static config.
	cm.processNewService(procSvc)
	assertConfigsMatch(t, published.last().Schedule)
	assertConfigsMatch(t, published.last().Unschedule)
	assertLoadedConfigsMatch(t, cm, matchAll(matchName("redis"), matchSvc("")))
}

// Static configs that share an integration name are refcounted in the
// index, so Has stays true through partial removals and only flips false
// when the last one goes away.
func TestStaticConfigIndexRefcountThroughConfigMgr(t *testing.T) {
	mockResolver := MockSecretResolver{}
	idx := listeners.NewStaticConfigIndex()

	cm := newReconcilingConfigManager(&mockResolver, nil, idx, nil, nil, noopConfigPublisher)

	staticRedis1 := integration.Config{Name: "redis", Instances: []integration.Data{integration.Data("port: 6379")}}
	staticRedis2 := integration.Config{Name: "redis", Instances: []integration.Data{integration.Data("port: 6380")}}

	cm.processNewConfig(staticRedis1)
	assert.True(t, idx.Has("redis"))

	cm.processNewConfig(staticRedis2)
	assert.True(t, idx.Has("redis"))

	cm.processDelConfigs([]integration.Config{staticRedis1})
	assert.True(t, idx.Has("redis"))

	cm.processDelConfigs([]integration.Config{staticRedis2})
	assert.False(t, idx.Has("redis"))
}

// Logs-only static configs (no Instances) are not added to the
// StaticConfigIndex. The index tracks integrations covered by a scheduled
// non-template *check* config so that dynamic templates of the same name can
// be deduplicated. A logs-only conf.d entry forwards logs but doesn't
// configure a check, so it must not suppress dynamic check templates.
func TestStaticConfigIndex_SkipsLogsOnlyConfigs(t *testing.T) {
	mockResolver := MockSecretResolver{}
	idx := listeners.NewStaticConfigIndex()

	cm := newReconcilingConfigManager(&mockResolver, nil, idx, nil, nil, noopConfigPublisher)

	logsOnly := integration.Config{
		Name:       "redis",
		LogsConfig: []byte(`{"source":"redis"}`),
	}
	cm.processNewConfig(logsOnly)
	assert.False(t, idx.Has("redis"),
		"logs-only static configs must not populate the static config index")

	// Removing the same config should be a no-op for the index.
	cm.processDelConfigs([]integration.Config{logsOnly})
	assert.False(t, idx.Has("redis"))
}

func TestResolveTemplateForService_ReportsToHealthPlatform(t *testing.T) {
	mockResolver := MockSecretResolver{}
	hp := healthplatformmock.New(t)

	cm := newReconcilingConfigManager(&mockResolver, hp, nil, nil, nil, noopConfigPublisher).(*reconcilingConfigManager)

	tpl := integration.Config{
		Name:          "postgres",
		ADIdentifiers: []string{"postgres"},
		Instances:     []integration.Data{integration.Data("host: %%host%%\ntags:\n  - dbid:%%extra_dbinstanceidentifier%%")},
		Provider:      "file",
		Source:        "file:/etc/datadog-agent/conf.d/postgres.d/conf.yaml",
	}

	svc := &dummyServiceWithExtraConfigError{
		dummyService: dummyService{
			ID:            "docker://abc123",
			ADIdentifiers: []string{"postgres"},
			Hosts:         map[string]string{"main": "myhost"},
		},
	}

	_, ok := cm.resolveTemplateForService(tpl, svc)
	assert.False(t, ok, "resolveTemplateForService should return false on resolution failure")

	count, issues := hp.GetAllIssues()
	assert.Equal(t, 1, count, "expected 1 health issue to be reported")
	expectedIssueID := admisconfig.TemplateIssueID + ":postgres:docker://abc123:" + tpl.Digest()
	issue := issues[expectedIssueID]
	require.NotNil(t, issue, "expected health issue at issue id %s", expectedIssueID)
	assert.Equal(t, expectedIssueID, issue.Id)
	assert.Equal(t, admisconfig.Source, issue.Source)
}

func TestResolveTemplateForService_ClearsHealthPlatformOnSuccess(t *testing.T) {
	mockResolver := MockSecretResolver{}
	hp := healthplatformmock.New(t)

	cm := newReconcilingConfigManager(&mockResolver, hp, nil, nil, nil, noopConfigPublisher).(*reconcilingConfigManager)

	tpl := integration.Config{
		Name:          "redis",
		ADIdentifiers: []string{"redis"},
		LogsConfig:    []byte("source: %%host%%"),
	}

	svc := &dummyService{
		ID:            "docker://def456",
		ADIdentifiers: []string{"redis"},
		Hosts:         map[string]string{"main": "myhost"},
	}

	// Pre-populate a health issue using the same IssueId format the code uses.
	hp.ReportIssue(&healthplatformpayload.Issue{
		Id:        admisconfig.TemplateIssueID + ":redis:docker://def456:" + tpl.Digest(),
		IssueName: admisconfig.TemplateIssueName,
		Source:    admisconfig.Source,
	})
	count, _ := hp.GetAllIssues()
	require.Equal(t, 1, count)

	_, ok := cm.resolveTemplateForService(tpl, svc)
	assert.True(t, ok)

	count, _ = hp.GetAllIssues()
	assert.Equal(t, 0, count, "health issue should be cleared after successful resolution")
}

func TestResolveTemplateForService_SkipsReportWhenServiceNotReady(t *testing.T) {
	mockResolver := MockSecretResolver{}
	hp := healthplatformmock.New(t)

	cm := newReconcilingConfigManager(&mockResolver, hp, nil, nil, nil, noopConfigPublisher).(*reconcilingConfigManager)

	tpl := integration.Config{
		Name:          "postgres",
		ADIdentifiers: []string{"postgres"},
		Instances:     []integration.Data{integration.Data("host: %%host%%")},
	}

	svc := &notReadyService{dummyService{
		ID:            "docker://abc123",
		ADIdentifiers: []string{"postgres"},
	}}

	_, ok := cm.resolveTemplateForService(tpl, svc)
	assert.False(t, ok, "a not-ready service must not resolve")

	count, _ := hp.GetAllIssues()
	assert.Equal(t, 0, count, "transient 'service not ready' must not be reported as a health issue")
}

// notReadyService exercises the transient configresolver.ErrServiceNotReady path.
type notReadyService struct {
	dummyService
}

func (notReadyService) IsReady() bool { return false }
