package orchestrator

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	agentmodel "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
)

func (suite *k8sSuite) TestPayloads() {
	expectAtLeastOneResource{
		filter: &fakeintake.PayloadFilter{ResourceType: agentmodel.TypeCollectorPod},
		test: func(payload *aggregator.OrchestratorPayload) bool {
			return strings.HasPrefix(payload.Pod.Metadata.Name, "redis-query") &&
				payload.Pod.Metadata.Namespace == "workload-redis"
		},
		message: "find a redis-query pod",
		timeout: time.Minute,
	}.Assert(suite)

	expectAtLeastOneManifest{
		test: func(payload *aggregator.OrchestratorManifestPayload) bool {
			if payload.Type != agentmodel.TypeCollectorManifest {
				return false
			}
			manif := map[string]any{}
			json.Unmarshal(payload.Manifest.Content, &manif)
			if _, ok := manif["metadata"]; !ok {
				return false
			}
			md := manif["metadata"].(map[string]any)
			return md["name"] == "redis" && md["namespace"] == "workload-redis"
		},
		message: "find a Deployment manifest",
		timeout: time.Minute,
	}.Assert(suite)

	/*
		TODO configure custom resources in chart
			expectAtLeastOneManifest{
				test: func(payload *aggregator.OrchestratorManifestPayload) bool {
					if payload.Type != agentmodel.TypeCollectorManifestCR {
						return false
					}
					manif := map[string]any{}
					json.Unmarshal(payload.Manifest.Content, &manif)
					return manif["kind"] == "DatadogMetric" && manif["apiVersion"] == "datadoghq.com/v1alpha1"
				},
				message: "find a DatadogMetric manifest",
				timeout: time.Minute,
			}.Assert(suite)
	*/
}

type expectAtLeastOneResource struct {
	filter  *fakeintake.PayloadFilter
	test    func(payload *aggregator.OrchestratorPayload) bool
	message string
	timeout time.Duration
}

func (e expectAtLeastOneResource) Assert(suite *k8sSuite) {
	giveup := time.Now().Add(e.timeout)
	fmt.Println("trying to " + e.message)
	for {
		payloads, err := suite.Fakeintake.GetOrchestratorResources(e.filter)
		suite.NoError(err, "failed to get resource payloads from intake")
		fmt.Printf("found %d resources\n", len(payloads))
		for _, p := range payloads {
			if p != nil && e.test(p) {
				return
			}
		}
		if time.Now().After(giveup) {
			break
		}
		time.Sleep(5 * time.Second)
	}
	suite.Fail("failed to " + e.message)
}

type expectAtLeastOneManifest struct {
	test    func(payload *aggregator.OrchestratorManifestPayload) bool
	message string
	timeout time.Duration
}

func (e expectAtLeastOneManifest) Assert(suite *k8sSuite) {
	giveup := time.Now().Add(e.timeout)
	fmt.Println("trying to " + e.message)
	for {
		payloads, err := suite.Fakeintake.GetOrchestratorManifests()
		suite.NoError(err, "failed to get manifest payloads from intake")
		fmt.Printf("found %d manifests\n", len(payloads))
		for _, p := range payloads {
			if p != nil && e.test(p) {
				return
			}
		}
		if time.Now().After(giveup) {
			break
		}
		time.Sleep(5 * time.Second)
	}
	suite.Fail("failed to " + e.message)
}
