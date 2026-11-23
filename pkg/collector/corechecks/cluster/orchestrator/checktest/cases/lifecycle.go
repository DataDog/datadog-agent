// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package cases

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/checktest/suite"
)

func init() { suite.Register("Lifecycle", lifecycle) }

// assignedPod is a pod that the check only knows from its deletion, as the pods collector lists unassigned pods.
var assignedPod = &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "assigned", Namespace: "default", UID: "assigned-pod", ResourceVersion: "1"},
	Spec: corev1.PodSpec{NodeName: "node"}}

// lifecycleCase lists, updates and deletes every fixture.
func lifecycleCase(variants []suite.Variant) suite.Case {
	// The variants run one after the other.
	var lastAssignedPod runtime.Object
	return suite.Case{Name: "Lifecycle", Variants: variants, Steps: []suite.Step{
		{Name: "first run", Want: leading(func(t *testing.T, sc *suite.Scenario) suite.ExpectedPayloads { return sc.LiveAll(t) })},
		{Name: "unchanged run", Want: suite.Nothing},
		{
			Name: "update every object",
			Act: func(t *testing.T, sc *suite.Scenario) {
				for _, o := range sc.Objects {
					updated := suite.UpdatedCopy(o.Object, "2")
					accessor, _ := meta.Accessor(updated)
					accessor.SetLabels(map[string]string{"updated": "true"})
					sc.Update(t, updated)
				}
			},
			Want: leading(func(t *testing.T, sc *suite.Scenario) suite.ExpectedPayloads {
				return sc.LiveOf(t, sc.Collected())
			}),
		},
		{
			Name: "delete every object",
			Act: func(t *testing.T, sc *suite.Scenario) {
				sc.Mocks.Cluster.Add(t, assignedPod.DeepCopy())
				for _, o := range sc.Objects {
					sc.Delete(t, o.Object)
				}
				lastAssignedPod = sc.Delete(t, assignedPod.DeepCopy())
				if sc.Mode != suite.ClusterAgentFollower {
					sc.WaitGone(t, sc.Objects)
				}
			},
			// Deleting the nodes changes the cluster.
			Want: leading(func(t *testing.T, sc *suite.Scenario) suite.ExpectedPayloads {
				return sc.TerminatedOf(t, sc.Collected()).With(sc.PayloadsOf(t, "v1/terminated-pods", lastAssignedPod, true), sc.ClusterPayloads())
			}),
			// The fake clients ignore the field selector of the unassigned pod informer, which can list the assigned
			// pod between its creation and its deletion.
			Ignore: func(s suite.SentResource) bool { return s.UID == string(assignedPod.UID) && !s.Terminated },
		},
	}}
}

// leading returns nothing for a follower, which sends nothing, and the payloads of want otherwise.
func leading(want func(t *testing.T, sc *suite.Scenario) suite.ExpectedPayloads) func(t *testing.T, sc *suite.Scenario) suite.ExpectedPayloads {
	return func(t *testing.T, sc *suite.Scenario) suite.ExpectedPayloads {
		if sc.Mode == suite.ClusterAgentFollower {
			return suite.ExpectedPayloads{}
		}
		return want(t, sc)
	}
}

// lifecycle runs the lifecycle in each deployment mode and configuration.
func lifecycle(t *testing.T, cs *suite.Suite) {
	var variants []suite.Variant
	for _, mode := range []suite.DeploymentMode{suite.ClusterAgentLeader, suite.ClusterAgentFollower, suite.ClusterCheckRunner} {
		for _, dedicatedPipeline := range []bool{false, true} {
			for _, allCollectors := range []bool{false, true} {
				variants = append(variants, suite.Variant{Mode: mode, Cfg: suite.ScenarioConfig{DedicatedPipeline: dedicatedPipeline, AllCollectors: allCollectors}})
			}
		}
	}
	// A cluster that only serves the older versions of the entries that have several.
	for _, dedicatedPipeline := range []bool{false, true} {
		variants = append(variants, suite.Variant{Mode: suite.ClusterAgentLeader, Cfg: suite.ScenarioConfig{DedicatedPipeline: dedicatedPipeline, OlderVersions: true}})
	}
	sent := cs.RunCase(t, lifecycleCase(variants))

	// The dedicated pipeline must not change the metadata models, which have no base. Removing the legacy pipeline
	// must replace this comparison, not skip it.
	compared := 0
	for key, steps := range sent {
		pair := strings.Replace(key, "pipeline=false", "pipeline=true", 1)
		if other, found := sent[pair]; found && pair != key && len(steps) > 0 && len(other) > 0 {
			assertSamePayloads(t, wholePayloads(t, steps[0]), wholePayloads(t, other[0]), key, pair)
			compared++
		}
	}
	assert.NotZero(t, compared, "no configuration is compared with the legacy pipeline")
}

// wholePayloads returns everything that a run sends for each object, by UID and payload type: the whole structured
// metadata model, and the whole manifest with its content decoded.
func wholePayloads(t *testing.T, sent []suite.SentResource) map[string]map[string]any {
	payloads := map[string]map[string]any{}
	for _, s := range sent {
		payloads[s.UID+"/"+map[bool]string{true: "metadata", false: "manifest"}[s.Metadata]] = suite.PayloadValue(t, s)
	}
	return payloads
}

// assertSamePayloads checks that two configurations send the same whole payloads for each object.
func assertSamePayloads(t *testing.T, expected, actual map[string]map[string]any, name, other string) {
	t.Helper()
	for key, payload := range expected {
		assert.Equal(t, payload, actual[key], "%s: %s and %s send different payloads", key, name, other)
	}
	for key := range actual {
		assert.Contains(t, expected, key, "%s: only %s sends it", key, other)
	}
}
