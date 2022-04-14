// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package autodiscovery

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/stretchr/testify/suite"
)

// changeMatch verifies that the given slice of changes has exactly
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

func matchName(name string) func(integration.Config) bool {
	return func(config integration.Config) bool {
		return config.Name == name
	}
}

func matchLogsConfig(logsConfig string) func(integration.Config) bool {
	return func(config integration.Config) bool {
		return string(config.LogsConfig) == logsConfig
	}
}

var (
	nonTemplateConfig = integration.Config{Name: "non-template"}
	templateConfig    = integration.Config{Name: "template", LogsConfig: []byte("source: %%host%%"), ADIdentifiers: []string{"my-service"}}
	myService         = &dummyService{ID: "abcd", ADIdentifiers: []string{"my-service"}, Hosts: map[string]string{"main": "myhost"}}
)

type SimpleConfigManagerSuite struct {
	suite.Suite
	cm configManager
}

func (suite *SimpleConfigManagerSuite) SetupTest() {
	suite.cm = newSimpleConfigManager()
}

// A new, non-template config is scheduled immediately and unscheduled when
// deleted
func (suite *SimpleConfigManagerSuite) TestNewNonTemplateScheduled() {
	changes := suite.cm.processNewConfig(nonTemplateConfig)
	assertConfigsMatch(suite.T(), changes.schedule, matchName("non-template"))
	assertConfigsMatch(suite.T(), changes.unschedule)

	changes = suite.cm.processDelConfigs([]integration.Config{nonTemplateConfig})
	assertConfigsMatch(suite.T(), changes.schedule)
	assertConfigsMatch(suite.T(), changes.unschedule, matchName("non-template"))
}

// A new template config is not scheduled when there is no matching service, and
// not unscheduled when removed
func (suite *SimpleConfigManagerSuite) TestNewTemplateNotScheduled() {
	changes := suite.cm.processNewConfig(templateConfig)
	assertConfigsMatch(suite.T(), changes.schedule)
	assertConfigsMatch(suite.T(), changes.unschedule)

	changes = suite.cm.processDelConfigs([]integration.Config{templateConfig})
	assertConfigsMatch(suite.T(), changes.schedule)
	assertConfigsMatch(suite.T(), changes.unschedule)
}

// A new template config is not scheduled when there is no matching service, but
// is resolved and scheduled when such a service arrives; deleting the config
// unschedules the resolved configs.
func (suite *SimpleConfigManagerSuite) TestNewTemplateBeforeService_ConfigRemovedFirst() {
	changes := suite.cm.processNewConfig(templateConfig)
	assertConfigsMatch(suite.T(), changes.schedule)
	assertConfigsMatch(suite.T(), changes.unschedule)

	changes = suite.cm.processNewService(myService.ADIdentifiers, myService)
	assertConfigsMatch(suite.T(), changes.schedule, matchAll(matchName("template"), matchLogsConfig("source: myhost")))
	assertConfigsMatch(suite.T(), changes.unschedule)

	changes = suite.cm.processDelConfigs([]integration.Config{templateConfig})
	assertConfigsMatch(suite.T(), changes.schedule)
	assertConfigsMatch(suite.T(), changes.unschedule, matchAll(matchName("template"), matchLogsConfig("source: myhost")))

	changes = suite.cm.processDelService(myService)
	assertConfigsMatch(suite.T(), changes.schedule)
	assertConfigsMatch(suite.T(), changes.unschedule)
}

// A new template config is not scheduled when there is no matching service, but
// is resolved and scheduled when such a service arrives; deleting the service
// unschedules the resolved configs.
func (suite *SimpleConfigManagerSuite) TestNewTemplateBeforeService_ServiceRemovedFirst() {
	changes := suite.cm.processNewConfig(templateConfig)
	assertConfigsMatch(suite.T(), changes.schedule)
	assertConfigsMatch(suite.T(), changes.unschedule)

	changes = suite.cm.processNewService(myService.ADIdentifiers, myService)
	assertConfigsMatch(suite.T(), changes.schedule, matchAll(matchName("template"), matchLogsConfig("source: myhost")))
	assertConfigsMatch(suite.T(), changes.unschedule)

	changes = suite.cm.processDelService(myService)
	assertConfigsMatch(suite.T(), changes.schedule)
	assertConfigsMatch(suite.T(), changes.unschedule, matchAll(matchName("template"), matchLogsConfig("source: myhost")))

	changes = suite.cm.processDelConfigs([]integration.Config{templateConfig})
	assertConfigsMatch(suite.T(), changes.schedule)
	assertConfigsMatch(suite.T(), changes.unschedule)
}

// A new service is not scheduled when there is no matching template, but
// is resolved and scheduled when such a template arrives; deleting the template
// unschedules the resolved configs.
func (suite *SimpleConfigManagerSuite) TestNewServiceBeforeTemplate_ConfigRemovedFirst() {
	changes := suite.cm.processNewService(myService.ADIdentifiers, myService)
	assertConfigsMatch(suite.T(), changes.schedule)
	assertConfigsMatch(suite.T(), changes.unschedule)

	changes = suite.cm.processNewConfig(templateConfig)
	assertConfigsMatch(suite.T(), changes.schedule, matchAll(matchName("template"), matchLogsConfig("source: myhost")))
	assertConfigsMatch(suite.T(), changes.unschedule)

	changes = suite.cm.processDelConfigs([]integration.Config{templateConfig})
	assertConfigsMatch(suite.T(), changes.schedule)
	assertConfigsMatch(suite.T(), changes.unschedule, matchAll(matchName("template"), matchLogsConfig("source: myhost")))

	changes = suite.cm.processDelService(myService)
	assertConfigsMatch(suite.T(), changes.schedule)
	assertConfigsMatch(suite.T(), changes.unschedule)
}

// A new service is not scheduled when there is no matching template, but
// is resolved and scheduled when such a template arrives; deleting the service
// unschedules the resolved configs.
func (suite *SimpleConfigManagerSuite) TestNewServiceBeforeTemplate_ServiceRemovedFirst() {
	changes := suite.cm.processNewService(myService.ADIdentifiers, myService)
	assertConfigsMatch(suite.T(), changes.schedule)
	assertConfigsMatch(suite.T(), changes.unschedule)

	changes = suite.cm.processNewConfig(templateConfig)
	assertConfigsMatch(suite.T(), changes.schedule, matchAll(matchName("template"), matchLogsConfig("source: myhost")))
	assertConfigsMatch(suite.T(), changes.unschedule)

	changes = suite.cm.processDelService(myService)
	assertConfigsMatch(suite.T(), changes.schedule)
	assertConfigsMatch(suite.T(), changes.unschedule, matchAll(matchName("template"), matchLogsConfig("source: myhost")))

	changes = suite.cm.processDelConfigs([]integration.Config{templateConfig})
	assertConfigsMatch(suite.T(), changes.schedule)
	assertConfigsMatch(suite.T(), changes.unschedule)
}

func TestSimpleConfigManagement(t *testing.T) {
	suite.Run(t, new(SimpleConfigManagerSuite))
}
