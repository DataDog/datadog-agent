// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubelet && kubeapiserver && test

package pod

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

// podScenarioConfig is the configuration of a pod check scenario.
type podScenarioConfig struct {
	manifestCollection bool
	dedicatedPipeline  bool
}

// TestPodCheckScenarios runs the pod check in each manifest configuration: a first run, an unchanged run, the update
// of a pod and the loss of the structured metadata cache. Each run must send each pod version once per payload type,
// and the manifests must not depend on the pipeline.
func (suite *PodTestSuite) TestPodCheckScenarios() {
	suite.setClusterID()
	saved := *suite.check.config
	suite.T().Cleanup(func() {
		*suite.check.config = saved
		suite.dummyKubelet.Lock()
		suite.dummyKubelet.PodsBody = nil
		suite.dummyKubelet.Unlock()
	})

	firstManifests := map[podScenarioConfig]map[string]string{}
	for _, cfg := range []podScenarioConfig{{false, false}, {true, false}, {true, true}} {
		suite.Run(fmt.Sprintf("manifests=%t,pipeline=%t", cfg.manifestCollection, cfg.dedicatedPipeline), func() {
			firstManifests[cfg] = suite.runPodScenario(cfg)
		})
	}
	assert.Equal(suite.T(), firstManifests[podScenarioConfig{true, false}], firstManifests[podScenarioConfig{true, true}],
		"the dedicated pipeline sends the same manifests")
}

func (suite *PodTestSuite) runPodScenario(cfg podScenarioConfig) map[string]string {
	t := suite.T()
	suite.SetupTest()
	suite.check.config.IsManifestCollectionEnabled = cfg.manifestCollection
	suite.check.config.ManifestPipelineEnabled = cfg.dedicatedPipeline
	pods := suite.podList()
	suite.servePods(pods)

	run := func() (metadata, manifests map[string]int) {
		kubelet.ResetCache()
		suite.sender.pods, suite.sender.manifests = nil, nil
		require.NoError(t, suite.check.Run())
		metadata, manifests = map[string]int{}, map[string]int{}
		for _, message := range suite.sender.pods {
			for _, pod := range message.(*process.CollectorPod).Pods {
				metadata[pod.Metadata.Uid]++
				assert.Equal(t, cfg.manifestCollection, len(pod.Yaml) == 0, "the metadata carries the YAML only without manifest collection")
			}
		}
		for _, message := range suite.sender.manifests {
			for _, manifest := range message.(*process.CollectorManifest).Manifests {
				manifests[manifest.Uid]++
			}
		}
		return metadata, manifests
	}
	noPods := map[string]int{}

	// First run: each pod is sent once, static pods with the UID that the check derives.
	metadata, manifests := run()
	allPods := metadata
	assert.Len(t, allPods, len(pods), "first run: metadata")
	for podUID, count := range allPods {
		assert.Equal(t, 1, count, "first run: metadata of %s", podUID)
	}
	manifestsOf := func(sent map[string]int) map[string]int {
		if cfg.manifestCollection {
			return sent
		}
		return noPods
	}
	assert.Equal(t, manifestsOf(allPods), manifests, "first run: manifests")
	first := suite.manifestContents()

	// An unchanged run sends nothing.
	metadata, manifests = run()
	assert.Equal(t, noPods, metadata, "unchanged run: metadata")
	assert.Equal(t, noPods, manifests, "unchanged run: manifests")

	// An updated pod is sent again, alone. The first pods of the fixture are static pods.
	updatedPod := pods[len(pods)-1]
	labels, _ := updatedPod["metadata"].(map[string]any)["labels"].(map[string]any)
	if labels == nil {
		labels = map[string]any{}
		updatedPod["metadata"].(map[string]any)["labels"] = labels
	}
	labels["updated"] = "true"
	suite.servePods(pods)
	updated := map[string]int{uid(updatedPod): 1}
	require.Contains(t, allPods, uid(updatedPod), "the updated pod is not a static pod")
	metadata, manifests = run()
	assert.Equal(t, updated, metadata, "update: metadata")
	assert.Equal(t, manifestsOf(updated), manifests, "update: manifests")

	// Losing the structured metadata cache resends the metadata. The dedicated pipeline deduplicates the manifests
	// independently.
	orchestrator.KubernetesResourceCache = orchestrator.NewKubernetesResourceCache()
	metadata, manifests = run()
	assert.Equal(t, allPods, metadata, "metadata cache loss: metadata")
	wantManifests := manifestsOf(allPods)
	if cfg.dedicatedPipeline {
		wantManifests = noPods
	}
	assert.Equal(t, wantManifests, manifests, "metadata cache loss: manifests")

	return first
}

// podList returns the pods of the kubelet fixture.
func (suite *PodTestSuite) podList() []map[string]any {
	body, err := os.ReadFile("../testdata/pods.json")
	require.NoError(suite.T(), err)
	var list struct {
		Items []map[string]any `json:"items"`
	}
	require.NoError(suite.T(), json.Unmarshal(body, &list))
	return list.Items
}

// servePods makes the kubelet serve the pods.
func (suite *PodTestSuite) servePods(pods []map[string]any) {
	body, err := json.Marshal(map[string]any{"kind": "PodList", "apiVersion": "v1", "items": pods})
	require.NoError(suite.T(), err)
	suite.dummyKubelet.Lock()
	defer suite.dummyKubelet.Unlock()
	suite.dummyKubelet.PodsBody = body
}

// manifestContents returns the content of the manifests of the last run, by UID.
func (suite *PodTestSuite) manifestContents() map[string]string {
	contents := map[string]string{}
	for _, message := range suite.sender.manifests {
		for _, manifest := range message.(*process.CollectorManifest).Manifests {
			contents[manifest.Uid] = string(manifest.Content)
		}
	}
	return contents
}

func uid(pod map[string]any) string {
	return pod["metadata"].(map[string]any)["uid"].(string)
}
