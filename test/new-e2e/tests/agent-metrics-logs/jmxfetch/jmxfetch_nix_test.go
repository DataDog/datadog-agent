// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package jmxfetch

import (
	_ "embed"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awsdocker "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/docker"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"

	"github.com/DataDog/test-infra-definitions/components/datadog/apps/jmxfetch"
	"github.com/DataDog/test-infra-definitions/components/datadog/dockeragentparams"
	"github.com/DataDog/test-infra-definitions/components/docker"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed testdata/docker-labels.yaml
var jmxFetchADLabels string

var jmxFetchADLabelsDockerComposeManifest = docker.ComposeInlineManifest{
	Name:    "jmx-test-app-labels",
	Content: pulumi.String(jmxFetchADLabels),
}

type jmxfetchNixTest struct {
	e2e.BaseSuite[environments.DockerHost]
}

func TestJMXFetchNix(t *testing.T) {
	t.Parallel()
	suiteParams := []e2e.SuiteOption{e2e.WithProvisioner(
		awsdocker.Provisioner(
			awsdocker.WithAgentOptions(
				dockeragentparams.WithLogs(),
				dockeragentparams.WithJMX(),
				dockeragentparams.WithExtraComposeInlineManifest(
					jmxfetch.DockerComposeManifest,
					jmxFetchADLabelsDockerComposeManifest,
				),
			)))}

	e2e.Run(t,
		&jmxfetchNixTest{},
		suiteParams...,
	)
}

func (j *jmxfetchNixTest) Test_FakeIntakeReceivesJMXFetchMetrics() {
	metricNames := []string{
		"test.e2e.jmxfetch.counter_100",
		"test.e2e.jmxfetch.gauge_200",
		"test.e2e.jmxfetch.increment_counter",
	}
	start := time.Now()
	j.EventuallyWithT(func(c *assert.CollectT) {
		for _, metricName := range metricNames {
			metrics, err := j.Env().FakeIntake.Client().
				FilterMetrics(metricName, client.WithMetricValueHigherThan(0))
			assert.NoError(c, err)
			assert.NotEmpty(j.T(), metrics, "no metrics found for", metricName)
		}
	}, 5*time.Minute, 10*time.Second)
	j.T().Logf("Started: %v and took %v", start, time.Since(start))

	// Helpful debug when things fail
	if j.T().Failed() {
		names, err := j.Env().FakeIntake.Client().GetMetricNames()
		assert.NoError(j.T(), err)
		for _, name := range names {
			j.T().Logf("Got metric: %q", name)
		}
		for _, metricName := range metricNames {
			tjc, err := j.Env().FakeIntake.Client().FilterMetrics(metricName)
			assert.NoError(j.T(), err)
			assert.NotEmpty(j.T(), tjc, "Filter metrics was empty for", metricName)
			if len(tjc) > 0 {
				for _, point := range tjc[0].Points {
					j.T().Logf("Found metrics: %q \n%v - %v \n%q", tjc[0].Metric, point, point.Value, tjc[0].Type)
				}
			}
		}
	}
}

func (j *jmxfetchNixTest) TestJMXListCollectedWithRateMetrics() {
	status, err := j.Env().Agent.Client.JMX(agentclient.WithArgs([]string{"list", "collected", "with-rate-metrics"}))
	require.NoError(j.T(), err)
	assert.NotEmpty(j.T(), status.Content)

	lines := strings.Split(status.Content, "\n")
	var consoleReporterOut []string
	var foundShouldBe100, foundShouldBe200, foundIncrementCounter bool
	for _, line := range lines {
		if strings.Contains(line, "ConsoleReporter") {
			consoleReporterOut = append(consoleReporterOut, line)
			if strings.Contains(line, "dd.test.sample:name=default,type=simple") {
				if strings.Contains(line, "ShouldBe100") {
					foundShouldBe100 = true
				}
				if strings.Contains(line, "ShouldBe200") {
					foundShouldBe200 = true
				}
				if strings.Contains(line, "IncrementCounter") {
					foundIncrementCounter = true
				}
			}
		}
	}

	assert.NotEmpty(j.T(), consoleReporterOut, "Did not find ConsoleReporter output in status")
	assert.True(j.T(), foundShouldBe100,
		"Did not find bean name: dd.test.sample:name=default,type=simple - Attribute name: ShouldBe100  - Attribute type: java.lang.Integer")
	assert.True(j.T(), foundShouldBe200,
		"Did not find bean name: dd.test.sample:name=default,type=simple - Attribute name: ShouldBe200  - Attribute type: java.lang.Double")
	assert.True(j.T(), foundIncrementCounter,
		"Did not find bean name: dd.test.sample:name=default,type=simple - Attribute name: IncrementCounter  - Attribute type: java.lang.Integer")

	// Helpful debug when things fail
	if j.T().Failed() {
		for _, line := range consoleReporterOut {
			j.T().Log(line)
		}
	}
}
