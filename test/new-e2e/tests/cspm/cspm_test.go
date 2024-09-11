// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package cspm contains the e2e tests for cspm
package cspm

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"slices"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/fields"

	"github.com/DataDog/test-infra-definitions/components/datadog/kubernetesagentparams"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awskubernetes "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/kubernetes"
)

type cspmTestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

type findings = map[string][]map[string]string

var expectedFindingsMasterEtcdNode = findings{
	"cis-kubernetes-1.5.1-1.1.12": []map[string]string{
		{
			"result": "failed",
		},
	},
	"cis-kubernetes-1.5.1-1.2.16": []map[string]string{
		{
			"result": "failed",
		},
	},
	"cis-kubernetes-1.5.1-1.2.19": []map[string]string{
		{
			"result": "failed",
		},
	},
	"cis-kubernetes-1.5.1-1.2.21": []map[string]string{
		{
			"result": "failed",
		},
	},
	"cis-kubernetes-1.5.1-1.2.22": []map[string]string{
		{
			"result": "failed",
		},
	},
	"cis-kubernetes-1.5.1-1.2.23": []map[string]string{
		{
			"result": "failed",
		},
	},
	"cis-kubernetes-1.5.1-1.2.24": []map[string]string{
		{
			"result": "failed",
		},
	},
	"cis-kubernetes-1.5.1-1.2.25": []map[string]string{
		{
			"result": "failed",
		},
	},
	"cis-kubernetes-1.5.1-1.2.26": []map[string]string{
		{
			"result": "failed",
		},
	},
	"cis-kubernetes-1.5.1-1.2.33": []map[string]string{
		{
			"result": "failed",
		},
	},
	"cis-kubernetes-1.5.1-1.2.6": []map[string]string{
		{
			"result": "failed",
		},
	},
	"cis-kubernetes-1.5.1-1.3.2": []map[string]string{
		{
			"result": "failed",
		},
	},
	"cis-kubernetes-1.5.1-1.3.3": []map[string]string{
		{
			"result": "passed",
		},
	},
	"cis-kubernetes-1.5.1-1.3.4": []map[string]string{
		{
			"result": "passed",
		},
	},
	"cis-kubernetes-1.5.1-1.3.5": []map[string]string{
		{
			"result": "passed",
		},
	},
	"cis-kubernetes-1.5.1-1.3.6": []map[string]string{
		{
			"result": "failed",
		},
	},
	"cis-kubernetes-1.5.1-1.3.7": []map[string]string{
		{
			"result": "passed",
		},
	},
	"cis-kubernetes-1.5.1-1.4.1": []map[string]string{
		{
			"result": "failed",
		},
	},
	"cis-kubernetes-1.5.1-3.2.1": []map[string]string{
		{
			"result": "failed",
		},
	},
}
var expectedFindingsWorkerNode = findings{
	"cis-kubernetes-1.5.1-4.2.1": []map[string]string{
		{
			"result": "failed",
		},
	},
	"cis-kubernetes-1.5.1-4.2.3": []map[string]string{
		{
			"result": "failed",
		},
	},
	"cis-kubernetes-1.5.1-4.2.4": []map[string]string{
		{
			"result": "failed",
		},
	},
	"cis-kubernetes-1.5.1-4.2.5": []map[string]string{
		{
			"result": "failed",
		},
	},
	"cis-kubernetes-1.5.1-4.2.6": []map[string]string{
		{
			"result": "failed",
		},
	},
	"cis-kubernetes-1.5.1-4.2.10": []map[string]string{
		{
			"result": "failed",
		},
	},
	"cis-kubernetes-1.5.1-4.2.12": []map[string]string{
		{
			"result": "failed",
		},
	},
}

//go:embed values.yaml
var values string

func TestCSPM(t *testing.T) {
	e2e.Run(t, &cspmTestSuite{}, e2e.WithProvisioner(awskubernetes.KindProvisioner(awskubernetes.WithAgentOptions(kubernetesagentparams.WithHelmValues(values), kubernetesagentparams.WithoutDualShipping()))))
}

func (s *cspmTestSuite) TestFindings() {
	res, err := s.Env().KubernetesCluster.Client().CoreV1().Pods("datadog").List(context.Background(), metav1.ListOptions{
		LabelSelector: fields.OneTermEqualSelector("app", s.Env().Agent.LinuxNodeAgent.LabelSelectors["app"]).String(),
	})
	assert.NoError(s.T(), err)
	assert.Len(s.T(), res.Items, 1)
	agentPod := res.Items[0]
	_, _, err = s.Env().KubernetesCluster.KubernetesClient.PodExec("datadog", agentPod.Name, "security-agent", []string{"security-agent", "compliance", "check", "--dump-reports", "/tmp/reports", "--report"})
	assert.NoError(s.T(), err)
	dumpContent, _, err := s.Env().KubernetesCluster.KubernetesClient.PodExec("datadog", agentPod.Name, "security-agent", []string{"cat", "/tmp/reports"})
	assert.NoError(s.T(), err)
	findings, err := parseFindingOutput(dumpContent)
	assert.NoError(s.T(), err)
	s.checkFindings(findings, mergeFindings(expectedFindingsMasterEtcdNode, expectedFindingsWorkerNode))
}

func (s *cspmTestSuite) TestMetrics() {
	s.T().Log("Waiting for datadog.security_agent.compliance.running metrics")
	assert.EventuallyWithT(s.T(), func(c *assert.CollectT) {

		metrics, err := s.Env().FakeIntake.Client().FilterMetrics("datadog.security_agent.compliance.running")
		assert.NoError(c, err)
		assert.NotEmpty(c, metrics)
		s.T().Log("Metrics found: datadog.security_agent.compliance.running")
	}, 2*time.Minute, 10*time.Second)

	s.T().Log("Waiting for datadog.security_agent.compliance.containers_running metrics")
	assert.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		metrics, err := s.Env().FakeIntake.Client().FilterMetrics("datadog.security_agent.compliance.containers_running")
		assert.NoError(c, err)
		assert.NotEmpty(c, metrics)
		s.T().Log("Metrics found: datadog.security_agent.compliance.containers_running")
	}, 2*time.Minute, 10*time.Second)

}
func (s *cspmTestSuite) checkFindings(findings, expectedFindings findings) {
	s.T().Helper()
	checkedRule := []string{}
	for expectedRule, expectedRuleFindinds := range expectedFindings {
		assert.Contains(s.T(), findings, expectedRule)
		for _, expectedFinding := range expectedRuleFindinds {
			found := false
			for _, finding := range findings[expectedRule] {
				if isSubset(expectedFinding, finding) {
					found = true
					break
				}
			}
			assert.Truef(s.T(), found, "unexpected finding %v  for rule %s", findings[expectedRule], expectedRule)
			checkedRule = append(checkedRule, expectedRule)
		}
	}
	for rule, ruleFindings := range findings {
		if slices.Contains(checkedRule, rule) {
			continue
		}
		for _, ruleFinding := range ruleFindings {
			fmt.Printf("rule %s finding %v\n", rule, ruleFinding["result"])
		}
	}
	for rule, ruleFindings := range findings {
		if slices.Contains(checkedRule, rule) {
			continue
		}
		for _, ruleFinding := range ruleFindings {
			assert.NotContains(s.T(), []string{"failed", "error"}, ruleFinding["result"], fmt.Sprintf("finding for rule %s not expected to be in failed or error state", rule))
		}
	}

}

func isSubset(a, b map[string]string) bool {
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func mergeFindings(a, b findings) findings {
	for k, v := range b {
		a[k] = v
	}
	return a
}

func parseFindingOutput(output string) (findings, error) {

	result := map[string]any{}
	parsedResult := findings{}
	err := json.Unmarshal([]byte(output), &result)
	if err != nil {
		return nil, err
	}
	for rule, ruleFindings := range result {
		ruleFindingsCasted, ok := ruleFindings.([]any)
		if !ok {
			return nil, fmt.Errorf("failed to parse output: %s for rule %s cannot be casted into []any", ruleFindings, rule)
		}
		parsedRuleFinding := []map[string]string{}
		for _, finding := range ruleFindingsCasted {
			findingCasted, ok := finding.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("failed to parse output: %s for rule %s cannot be casted into map[string]any", finding, rule)
			}
			parsedFinding := map[string]string{}
			for k, v := range findingCasted {
				if _, ok := v.(string); ok {
					parsedFinding[k] = v.(string)
				}
			}
			parsedRuleFinding = append(parsedRuleFinding, parsedFinding)

		}
		parsedResult[rule] = parsedRuleFinding

	}
	return parsedResult, nil
}
