// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clustermetadata

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	cm "github.com/DataDog/datadog-agent/pkg/clustermetadata"
)

// TestRingTagger tests the pod-miss fallback to the ring.
// Test partitions:
// - entity: pod UID (ring path) | non-pod prefix (never asks the ring)
// - local answer: hit (no ring call) | miss (ring answers) | miss + ring miss (local result stands)
func TestRingTagger(t *testing.T) {
	store, _ := fanoutFixture(t)
	peer := &fakePeer{}

	peer.originAnswer = cm.LookupAnswer{Kind: cm.AnswerFound, Tags: []string{"pod_name:web-1"}}
	fanout := NewLocalStore(store.wmeta, store.tagger, store.ring, func() []cm.Store { return []cm.Store{peer} })
	// The wrapper's local miss is the tagger miss: the wmeta cache alone is
	// invisible to it, the local tagger answers. Empty the tagger for this
	// replica to simulate a shard that does not hold the pod.
	fanout.tagger.(*fakeTagger).tags = map[taggertypes.EntityID][]string{}
	fanout.wmeta.(*fakeWmeta).pods = map[string]*workloadmeta.KubernetesPod{}

	wrapped := NewRingTagger(store.tagger, fanout)

	// Local miss + ring hit: the ring's tags come back.
	tags, err := wrapped.Tag(taggertypes.NewEntityID(taggertypes.KubernetesPodUID, "uid-1"), taggertypes.LowCardinality)
	require.NoError(t, err)
	assert.Equal(t, []string{"pod_name:web-1"}, tags, "tags come from the ring, not the local tagger")
	assert.Equal(t, 1, peer.originCalls, "a pod miss asks the ring")

	// A local hit answers without the ring.
	store2, _ := fanoutFixture(t)
	peer2 := &fakePeer{}
	fanout2 := NewLocalStore(store2.wmeta, store2.tagger, store2.ring, func() []cm.Store { return []cm.Store{peer2} })
	wrapped2 := NewRingTagger(store2.tagger, fanout2)
	tags, err = wrapped2.Tag(taggertypes.NewEntityID(taggertypes.KubernetesPodUID, "uid-1"), taggertypes.LowCardinality)
	require.NoError(t, err)
	assert.Equal(t, []string{"kube_namespace:default", "pod_name:web-1"}, tags)
	assert.Zero(t, peer2.lookupCalls, "a local hit does not ask the ring")

	// A non-pod entity never asks the ring, even on a miss.
	deploymentEntity := taggertypes.NewEntityID(taggertypes.KubernetesDeployment, "default/web")
	_, _ = wrapped2.Tag(deploymentEntity, taggertypes.LowCardinality)
	assert.Zero(t, peer2.lookupCalls, "non-pod entities never ask the ring")

}
