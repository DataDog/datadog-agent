// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package autodiscoveryimpl

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"github.com/mohae/deepcopy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/listeners"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/util/testutil"
)

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
	factory func() configManager
	cm      configManager
}

func (suite *ConfigManagerSuite) SetupTest() {
	suite.cm = suite.factory()
}

// A new, non-template config is scheduled immediately and unscheduled when
// deleted
func (suite *ConfigManagerSuite) TestNewNonTemplateScheduled() {
	changes, _ := suite.cm.processNewConfig(nonTemplateConfig)
	assertConfigsMatch(suite.T(), changes.Schedule, matchName("non-template"))
	assertConfigsMatch(suite.T(), changes.Unschedule)

	changes = suite.cm.processDelConfigs([]integration.Config{nonTemplateConfig})
	assertConfigsMatch(suite.T(), changes.Schedule)
	assertConfigsMatch(suite.T(), changes.Unschedule, matchName("non-template"))
}

// A new, non-template config with secrets is scheduled immediately and unscheduled when
// deleted
func (suite *ConfigManagerSuite) TestNewNonTemplateWithSecretsScheduled() {
	mockResolver := MockSecretResolver{suite.T(), []mockSecretScenario{
		{
			expectedData:   []byte("foo: ENC[bar]"),
			expectedOrigin: nonTemplateConfigWithSecrets.Name,
			returnedData:   []byte("foo: barDecoded"),
			returnedError:  nil,
		},
		{
			expectedData:   []byte{},
			expectedOrigin: nonTemplateConfigWithSecrets.Name,
			returnedData:   []byte{},
			returnedError:  nil,
		},
	}}
	// assign mock secretResolver to the configManager
	cm := suite.cm.(*reconcilingConfigManager)
	cm.secretResolver = &mockResolver

	inputNewConfig := deepcopy.Copy(nonTemplateConfigWithSecrets).(integration.Config)
	changes, changedIDs := suite.cm.processNewConfig(inputNewConfig)

	assert.Empty(suite.T(), changedIDs) // Only returned if the config provider is cluster-checks.

	assertConfigsMatch(suite.T(), changes.Schedule, matchName(nonTemplateConfigWithSecrets.Name))
	assertConfigsMatch(suite.T(), changes.Unschedule)
	// Verify content is actually decoded

	require.True(suite.T(), strings.Contains(string(changes.Schedule[0].Instances[0]), "barDecoded"))
	newConfigDigest := changes.Schedule[0].Digest()

	inputDelConfig := deepcopy.Copy(nonTemplateConfigWithSecrets).(integration.Config)
	changes = suite.cm.processDelConfigs([]integration.Config{inputDelConfig})
	assertConfigsMatch(suite.T(), changes.Schedule)
	assertConfigsMatch(suite.T(), changes.Unschedule, matchName(nonTemplateConfigWithSecrets.Name))
	assertConfigsMatch(suite.T(), changes.Unschedule, matchDigest(newConfigDigest))
	require.True(suite.T(), strings.Contains(string(changes.Unschedule[0].Instances[0]), "barDecoded"))
}

func (suite *ConfigManagerSuite) TestNewClusterCheckWithSecretsScheduled() {
	mockResolver := MockSecretResolver{suite.T(), []mockSecretScenario{
		{
			expectedData:   []byte("foo: ENC[bar]"),
			expectedOrigin: clusterCheckConfigWithSecrets.Name,
			returnedData:   []byte("foo: barDecoded"),
			returnedError:  nil,
		},
		{
			expectedData:   []byte{},
			expectedOrigin: clusterCheckConfigWithSecrets.Name,
			returnedData:   []byte{},
			returnedError:  nil,
		},
	}}
	// assign mock secretResolver to the configManager
	cm := suite.cm.(*reconcilingConfigManager)
	cm.secretResolver = &mockResolver

	inputNewConfig := deepcopy.Copy(clusterCheckConfigWithSecrets).(integration.Config)
	changes, changedIDs := suite.cm.processNewConfig(inputNewConfig)

	// Check that changedIDs contains the correct mapping of IDs
	originalCheckID := checkid.BuildID(
		clusterCheckConfigWithSecrets.Name,
		clusterCheckConfigWithSecrets.FastDigest(),
		clusterCheckConfigWithSecrets.Instances[0],
		clusterCheckConfigWithSecrets.InitConfig,
	)
	newCheckID := checkid.BuildID(
		changes.Schedule[0].Name,
		changes.Schedule[0].FastDigest(),
		changes.Schedule[0].Instances[0],
		changes.Schedule[0].InitConfig,
	)
	assert.Equal(suite.T(), map[checkid.ID]checkid.ID{newCheckID: originalCheckID}, changedIDs)

	assertConfigsMatch(suite.T(), changes.Schedule, matchName(clusterCheckConfigWithSecrets.Name))
	assertConfigsMatch(suite.T(), changes.Unschedule)
	// Verify content is actually decoded
	require.True(suite.T(), strings.Contains(string(changes.Schedule[0].Instances[0]), "barDecoded"))
	newConfigDigest := changes.Schedule[0].Digest()

	inputDelConfig := deepcopy.Copy(clusterCheckConfigWithSecrets).(integration.Config)
	changes = suite.cm.processDelConfigs([]integration.Config{inputDelConfig})
	assertConfigsMatch(suite.T(), changes.Schedule)
	assertConfigsMatch(suite.T(), changes.Unschedule, matchName(clusterCheckConfigWithSecrets.Name))
	assertConfigsMatch(suite.T(), changes.Unschedule, matchDigest(newConfigDigest))
	require.True(suite.T(), strings.Contains(string(changes.Unschedule[0].Instances[0]), "barDecoded"))
}

// A new template config is not scheduled when there is no matching service, and
// not unscheduled when removed
func (suite *ConfigManagerSuite) TestNewTemplateNotScheduled() {
	changes, _ := suite.cm.processNewConfig(templateConfig)
	assertConfigsMatch(suite.T(), changes.Schedule)
	assertConfigsMatch(suite.T(), changes.Unschedule)

	changes = suite.cm.processDelConfigs([]integration.Config{templateConfig})
	assertConfigsMatch(suite.T(), changes.Schedule)
	assertConfigsMatch(suite.T(), changes.Unschedule)
}

// A new template config is not scheduled when there is no matching service, but
// is resolved and scheduled when such a service arrives; deleting the config
// unschedules the resolved configs.
func (suite *ConfigManagerSuite) TestNewTemplateBeforeService_ConfigRemovedFirst() {
	changes, _ := suite.cm.processNewConfig(templateConfig)
	assertConfigsMatch(suite.T(), changes.Schedule)
	assertConfigsMatch(suite.T(), changes.Unschedule)

	changes = suite.cm.processNewService(myService.ADIdentifiers, myService)
	assertConfigsMatch(suite.T(), changes.Schedule, matchAll(matchName("template"), matchLogsConfig("source: myhost\n")))
	assertConfigsMatch(suite.T(), changes.Unschedule)

	changes = suite.cm.processDelConfigs([]integration.Config{templateConfig})
	assertConfigsMatch(suite.T(), changes.Schedule)
	assertConfigsMatch(suite.T(), changes.Unschedule, matchAll(matchName("template"), matchLogsConfig("source: myhost\n")))

	changes = suite.cm.processDelService(context.TODO(), myService)
	assertConfigsMatch(suite.T(), changes.Schedule)
	assertConfigsMatch(suite.T(), changes.Unschedule)
}

// A new template config is not scheduled when there is no matching service, but
// is resolved and scheduled when such a service arrives; deleting the service
// unschedules the resolved configs.
func (suite *ConfigManagerSuite) TestNewTemplateBeforeService_ServiceRemovedFirst() {
	changes, _ := suite.cm.processNewConfig(templateConfig)
	assertConfigsMatch(suite.T(), changes.Schedule)
	assertConfigsMatch(suite.T(), changes.Unschedule)

	changes = suite.cm.processNewService(myService.ADIdentifiers, myService)
	assertConfigsMatch(suite.T(), changes.Schedule, matchAll(matchName("template"), matchLogsConfig("source: myhost\n")))
	assertConfigsMatch(suite.T(), changes.Unschedule)

	changes = suite.cm.processDelService(context.TODO(), myService)
	assertConfigsMatch(suite.T(), changes.Schedule)
	assertConfigsMatch(suite.T(), changes.Unschedule, matchAll(matchName("template"), matchLogsConfig("source: myhost\n")))

	changes = suite.cm.processDelConfigs([]integration.Config{templateConfig})
	assertConfigsMatch(suite.T(), changes.Schedule)
	assertConfigsMatch(suite.T(), changes.Unschedule)
}

// A new service is not scheduled when there is no matching template, but
// is resolved and scheduled when such a template arrives; deleting the template
// unschedules the resolved configs.
func (suite *ConfigManagerSuite) TestNewServiceBeforeTemplate_ConfigRemovedFirst() {
	changes := suite.cm.processNewService(myService.ADIdentifiers, myService)
	assertConfigsMatch(suite.T(), changes.Schedule)
	assertConfigsMatch(suite.T(), changes.Unschedule)

	changes, _ = suite.cm.processNewConfig(templateConfig)
	assertConfigsMatch(suite.T(), changes.Schedule, matchAll(matchName("template"), matchLogsConfig("source: myhost\n")))
	assertConfigsMatch(suite.T(), changes.Unschedule)

	changes = suite.cm.processDelConfigs([]integration.Config{templateConfig})
	assertConfigsMatch(suite.T(), changes.Schedule)
	assertConfigsMatch(suite.T(), changes.Unschedule, matchAll(matchName("template"), matchLogsConfig("source: myhost\n")))

	changes = suite.cm.processDelService(context.TODO(), myService)
	assertConfigsMatch(suite.T(), changes.Schedule)
	assertConfigsMatch(suite.T(), changes.Unschedule)
}

// A new service is not scheduled when there is no matching template, but
// is resolved and scheduled when such a template arrives; deleting the service
// unschedules the resolved configs.
func (suite *ConfigManagerSuite) TestNewServiceBeforeTemplate_ServiceRemovedFirst() {
	changes := suite.cm.processNewService(myService.ADIdentifiers, myService)
	assertConfigsMatch(suite.T(), changes.Schedule)
	assertConfigsMatch(suite.T(), changes.Unschedule)

	changes, _ = suite.cm.processNewConfig(templateConfig)
	assertConfigsMatch(suite.T(), changes.Schedule, matchAll(matchName("template"), matchLogsConfig("source: myhost\n")))
	assertConfigsMatch(suite.T(), changes.Unschedule)

	changes = suite.cm.processDelService(context.TODO(), myService)
	assertConfigsMatch(suite.T(), changes.Schedule)
	assertConfigsMatch(suite.T(), changes.Unschedule, matchAll(matchName("template"), matchLogsConfig("source: myhost\n")))

	changes = suite.cm.processDelConfigs([]integration.Config{templateConfig})
	assertConfigsMatch(suite.T(), changes.Schedule)
	assertConfigsMatch(suite.T(), changes.Unschedule)
}

// Fuzz the config manager to ensure it doesn't "leak" configs -- that schedule
// and unschedule calls are always properly paired.
func (suite *ConfigManagerSuite) TestFuzz() {
	testutil.Fuzz(suite.T(), func(seed int64) {
		fmt.Printf("==== starting fuzz with random seed %d\n", seed)
		cm := suite.factory()
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
				adIDs, _ := svc.GetADIdentifiers(context.Background())
				if _, found := services[id]; !found {
					services[id] = svc
					fmt.Printf("add service %s with AD idents [%s]\n", id, strings.Join(adIDs, ", "))
					applyChanges(cm.processNewService(adIDs, svc))
				}
			case p < 40 && op < removeAfterOps: // add non-template config
				cfg := makeNonTemplateConfig(r)
				digest := cfg.Digest()
				if _, found := configs[digest]; !found {
					configs[digest] = cfg
					fmt.Printf("add non-template config %s (digest %s)\n", cfg.Name, digest)
					changes, _ := cm.processNewConfig(cfg)
					applyChanges(changes)
				}
			case p < 60 && op < removeAfterOps: // add template config
				cfg := makeTemplateConfig(r)
				digest := cfg.Digest()
				if _, found := configs[digest]; !found {
					configs[digest] = cfg
					fmt.Printf("add template config %s (digest %s) with AD idents [%s]\n",
						cfg.Name, digest, strings.Join(cfg.ADIdentifiers, ", "))
					changes, _ := cm.processNewConfig(cfg)
					applyChanges(changes)
				}
			case p < 70 && len(services) > 0: // remove service
				i := rand.Intn(len(services))
				for id, svc := range services {
					if i == 0 {
						delete(services, id)
						adIDs, _ := svc.GetADIdentifiers(context.Background())
						fmt.Printf("remove service %s with AD idents %s\n", id, strings.Join(adIDs, ", "))
						applyChanges(cm.processDelService(context.TODO(), svc))
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
						applyChanges(cm.processDelConfigs([]integration.Config{cfg}))
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
	changes := suite.cm.processNewService(myService.ADIdentifiers, myService)
	assertConfigsMatch(suite.T(), changes.Schedule)
	assertConfigsMatch(suite.T(), changes.Unschedule)
	assertLoadedConfigsMatch(suite.T(), suite.cm)

	// adding service with no templates has no effect
	changes = suite.cm.processNewService(filterSvc.ADIdentifiers, filterSvc)
	assertConfigsMatch(suite.T(), changes.Schedule)
	assertConfigsMatch(suite.T(), changes.Unschedule)
	assertLoadedConfigsMatch(suite.T(), suite.cm)

	// adding a template that does not end in -keep only matches my-service
	cfg1 := integration.Config{Name: "cfg1", ADIdentifiers: []string{"my-service", "filter"}}
	changes, _ = suite.cm.processNewConfig(cfg1)
	assertConfigsMatch(suite.T(), changes.Schedule, matchAll(matchName("cfg1"), matchSvc("my-service")))
	assertConfigsMatch(suite.T(), changes.Unschedule)
	assertLoadedConfigsMatch(suite.T(), suite.cm, matchAll(matchName("cfg1"), matchSvc("my-service")))

	// adding a template that ends in -keep matches both services
	cfg2 := integration.Config{Name: "cfg2-keep", ADIdentifiers: []string{"my-service", "filter"}}
	changes, _ = suite.cm.processNewConfig(cfg2)
	assertConfigsMatch(suite.T(), changes.Schedule,
		matchAll(matchName("cfg2-keep"), matchSvc("my-service")),
		matchAll(matchName("cfg2-keep"), matchSvc("filter")),
	)
	assertConfigsMatch(suite.T(), changes.Unschedule)
	assertLoadedConfigsMatch(suite.T(), suite.cm,
		matchAll(matchName("cfg1"), matchSvc("my-service")),
		matchAll(matchName("cfg2-keep"), matchSvc("my-service")),
		matchAll(matchName("cfg2-keep"), matchSvc("filter")),
	)

	// removing a service removes only the scheduled configs
	changes = suite.cm.processDelService(context.TODO(), filterSvc)
	assertConfigsMatch(suite.T(), changes.Schedule)
	assertConfigsMatch(suite.T(), changes.Unschedule, matchAll(matchName("cfg2-keep"), matchSvc("filter")))
	assertLoadedConfigsMatch(suite.T(), suite.cm,
		matchAll(matchName("cfg1"), matchSvc("my-service")),
		matchAll(matchName("cfg2-keep"), matchSvc("my-service")),
	)
}

func TestReconcilingConfigManagement(t *testing.T) {
	mockResolver := MockSecretResolver{}
	suite.Run(t, &ReconcilingConfigManagerSuite{
		ConfigManagerSuite{factory: func() configManager {
			return newReconcilingConfigManager(&mockResolver)
		}},
	})
}
