// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package v1

import (
	"fmt"
	"runtime"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/sets"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

func deployment(name string) kubernetes.WorkloadTarget {
	return kubernetes.WorkloadTarget{Kind: kubernetes.DeploymentKind, Namespace: "ns", Name: name}
}

// assertEmpty checks that nothing is left behind once every pod is gone.
func assertEmpty(t *testing.T, idx *placementIndex) {
	t.Helper()
	assert.Empty(t, idx.pods)
	assert.Empty(t, idx.counts)
	assert.Zero(t, idx.nodes.len())
	assert.Zero(t, idx.workloads.len())
}

func TestPlacementIndexCounts(t *testing.T) {
	app, other := deployment("app"), deployment("other")
	idx := newPlacementIndex()

	changes := idx.set("pod-1", "node-a", app, nil)
	assert.Equal(t, []hostingChange{{node: "node-a", target: app}}, changes, "first pod of app on node-a")

	assert.Empty(t, idx.set("pod-2", "node-a", app, nil), "app already on node-a")
	assert.Empty(t, idx.set("pod-2", "node-a", app, nil), "same placement again (status update)")
	assert.Equal(t, []hostingChange{{node: "node-b", target: app}}, idx.set("pod-3", "node-b", app, nil))
	assert.Equal(t, []hostingChange{{node: "node-b", target: other}}, idx.set("pod-4", "node-b", other, nil))

	assert.True(t, idx.hosts(app, "node-a"))
	assert.True(t, idx.hosts(app, "node-b"))
	assert.False(t, idx.hosts(other, "node-a"))
	assert.ElementsMatch(t, []string{"node-a", "node-b"}, slices.Collect(idx.nodesOf(app)))
	assert.Equal(t, 2, idx.nodes.len())
	assert.Equal(t, 2, idx.workloads.len())

	assert.Empty(t, idx.delete("pod-1", nil), "pod-2 is still on node-a")
	assert.Equal(t, []hostingChange{{node: "node-a", target: app}}, idx.delete("pod-2", nil))
	assert.False(t, idx.hosts(app, "node-a"))
	assert.Empty(t, idx.delete("pod-2", nil), "unknown pod")

	idx.delete("pod-3", nil)
	idx.delete("pod-4", nil)
	assertEmpty(t, idx)
}

func TestPlacementIndexMove(t *testing.T) {
	app, other := deployment("app"), deployment("other")
	idx := newPlacementIndex()
	idx.set("pod-1", "node-a", app, nil)
	idx.set("pod-2", "node-a", app, nil)

	// Adoption: owner references are mutable, so the same pod can move to
	// another workload. Its count must follow, or the old workload would stay
	// on the node forever.
	changes := idx.set("pod-2", "node-a", other, nil)
	assert.Equal(t, []hostingChange{{node: "node-a", target: other}}, changes, "app keeps pod-1 on node-a")
	changes = idx.set("pod-1", "node-a", other, nil)
	assert.Equal(t, []hostingChange{{node: "node-a", target: app}}, changes, "app's last pod moved away")
	assert.False(t, idx.hosts(app, "node-a"))
	assert.True(t, idx.hosts(other, "node-a"))
	assert.Equal(t, 1, idx.workloads.len(), "app's ID is released")

	idx.delete("pod-1", nil)
	idx.delete("pod-2", nil)
	assertEmpty(t, idx)
}

func TestPlacementIndexReusesIDs(t *testing.T) {
	idx := newPlacementIndex()
	idx.set("pod-1", "node-a", deployment("app"), nil)
	appID, _ := idx.workloads.lookup(deployment("app"))
	idx.delete("pod-1", nil)

	idx.set("pod-2", "node-a", deployment("other"), nil)
	otherID, _ := idx.workloads.lookup(deployment("other"))
	assert.Equal(t, appID, otherID, "the freed ID is reused")
	assert.False(t, idx.hosts(deployment("app"), "node-a"), "a reused ID does not resolve to its previous value")
	assert.Len(t, idx.workloads.values, 1, "the table does not grow with churn")
}

func TestPlacementIndexLookupsDoNotIntern(t *testing.T) {
	idx := newPlacementIndex()
	idx.set("pod-1", "node-a", deployment("app"), nil)

	assert.False(t, idx.hosts(deployment("unknown"), "node-a"))
	assert.False(t, idx.hosts(deployment("app"), "unknown-node"))
	assert.Empty(t, slices.Collect(idx.nodesOf(deployment("unknown"))))
	assert.Equal(t, 1, idx.nodes.len())
	assert.Equal(t, 1, idx.workloads.len())
}

// TestProcessPodEventsFollowsWorkloadChanges checks that the stream server
// moves a pod when the workload it resolves to changes, here from Deployment
// to Rollout when the pod gains the Argo Rollouts label.
func TestProcessPodEventsFollowsWorkloadChanges(t *testing.T) {
	srv := NewKubeMetadataStreamServer(nil, nil)
	app := deployment("app")
	rollout := kubernetes.WorkloadTarget{Kind: kubernetes.RolloutKind, Namespace: "ns", Name: "app"}
	srv.metadata.workloadAutoscalers[app] = sets.New("hpa")
	srv.metadata.workloadAutoscalers[rollout] = sets.New("dpa")

	pod := func(labels map[string]string) workloadmeta.Event {
		return workloadmeta.Event{Type: workloadmeta.EventTypeSet, Entity: &workloadmeta.KubernetesPod{
			EntityID:   workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesPod, ID: "pod-1"},
			EntityMeta: workloadmeta.EntityMeta{Name: "pod-1", Namespace: "ns", Labels: labels},
			Owners:     []workloadmeta.KubernetesPodOwner{{Kind: kubernetes.ReplicaSetKind, Name: "app-7d9f8b6c5d"}},
			NodeName:   "node-a",
		}}
	}

	srv.processPodEvents([]workloadmeta.Event{pod(nil)})
	assert.True(t, srv.placements.hosts(app, "node-a"))

	srv.processPodEvents([]workloadmeta.Event{pod(map[string]string{kubernetes.ArgoRolloutLabelKey: "7d9f8b6c5d"})})
	assert.False(t, srv.placements.hosts(app, "node-a"), "the Deployment no longer has a pod on node-a")
	assert.True(t, srv.placements.hosts(rollout, "node-a"))

	srv.processPodEvents([]workloadmeta.Event{{Type: workloadmeta.EventTypeUnset, Entity: pod(nil).Entity}})
	assertEmpty(t, srv.placements)
}

// BenchmarkPlacementIndexMemory reports the index's heap cost per pod at the
// scale of a large cluster: 100k pods, 10k workloads, 1,000 nodes.
func BenchmarkPlacementIndexMemory(b *testing.B) {
	const pods, workloads, nodes = 100_000, 10_000, 1_000

	// Pod UIDs and node names are owned by workloadmeta entities in
	// production: build them outside of the measurement.
	uids := make([]string, pods)
	for i := range uids {
		uids[i] = fmt.Sprintf("%08x-0000-4000-8000-%012x", i, i)
	}
	nodeNames := make([]string, nodes)
	for i := range nodeNames {
		nodeNames[i] = fmt.Sprintf("ip-10-0-%d-%d.ec2.internal", i/256, i%256)
	}
	targets := make([]kubernetes.WorkloadTarget, workloads)
	for i := range targets {
		targets[i] = kubernetes.WorkloadTarget{Kind: kubernetes.DeploymentKind, Namespace: fmt.Sprintf("team-%03d", i%100), Name: fmt.Sprintf("service-%05d", i)}
	}

	var bytesPerPod float64
	for b.Loop() {
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)

		idx := newPlacementIndex()
		for i, uid := range uids {
			// Pods of a workload spread over consecutive nodes, 10 per workload.
			idx.set(uid, nodeNames[(i/workloads+i)%nodes], targets[i%workloads], nil)
		}

		runtime.GC()
		runtime.ReadMemStats(&after)
		require.Len(b, idx.pods, pods)
		bytesPerPod = float64(after.HeapAlloc-before.HeapAlloc) / pods
		runtime.KeepAlive(idx)
	}
	b.ReportMetric(bytesPerPod, "B/pod")
}
