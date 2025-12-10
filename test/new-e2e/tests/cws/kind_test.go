// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cws

import (
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/new-e2e/tests/cws/api"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
)

const (
	k8sHostnamePrefix = "cws-e2e-kind-node"
	osPlatform        = "ubuntu"
	osArch            = "x86_64"
	osVersion         = "22-04"
)

type kindSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
	apiClient  *api.Client
	ddHostname string
}

func (s *kindSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	s.apiClient = api.NewClient()
}

func (s *kindSuite) Hostname() string {
	return s.ddHostname
}

func (s *kindSuite) Client() *api.Client {
	return s.apiClient
}

func (s *kindSuite) Test00RulesetLoadedDefaultFile() {
	assert.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		testRulesetLoaded(c, s, "file", "default.policy")
	}, 1*time.Minute, 5*time.Second)
}

func (s *kindSuite) Test01RulesetLoadedDefaultRC() {
	assert.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		testRulesetLoaded(c, s, "remote-config", "threat-detection.policy")
	}, 1*time.Minute, 5*time.Second)
}

func (s *kindSuite) Test02Selftests() {
	assert.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		testSelftestsEvent(c, s, func(event *api.SelftestsEvent) {
			assert.Contains(c, event.SucceededTests, "datadog_agent_cws_self_test_rule_open", "missing selftest result")
			assert.Contains(c, event.SucceededTests, "datadog_agent_cws_self_test_rule_chmod", "missing selftest result")
			assert.Contains(c, event.SucceededTests, "datadog_agent_cws_self_test_rule_chown", "missing selftest result")
			validateEventSchema(c, &event.Event, "self_test_schema.json")
		})
	}, 1*time.Minute, 5*time.Second)
}

func (s *kindSuite) Test03MetricRuntimeRunning() {
	assert.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		testMetricExists(c, s, "datadog.security_agent.runtime.running", map[string]string{"host": s.Hostname()})
	}, 2*time.Minute, 10*time.Second)
}

func (s *kindSuite) Test04MetricContainersRunning() {
	assert.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		testMetricExists(c, s, "datadog.security_agent.runtime.containers_running", map[string]string{"host": s.Hostname()})
	}, 2*time.Minute, 10*time.Second)
}

// test that the detection of CWS is properly working
// this test can be quite long so run it last
func (s *kindSuite) Test99CWSEnabled() {
	assert.EventuallyWithTf(s.T(), func(c *assert.CollectT) {
		testCwsEnabled(c, s)
	}, 20*time.Minute, 30*time.Second, "cws activation test timed out for host %s", s.Hostname())
}
