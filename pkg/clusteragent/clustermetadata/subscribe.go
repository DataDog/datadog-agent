// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clustermetadata

import (
	"context"
	"fmt"
	"sync"

	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	cm "github.com/DataDog/datadog-agent/pkg/clustermetadata"
)

// streamBufferSize bounds the event channel. The pump paces itself to the
// consumer through this buffer and the done channel, never dropping events.
const streamBufferSize = 128

// Subscribe implements cm.Store.
//
// v1 semantics:
//   - this replica must own the node; a non-owner refuses — the consumer
//     re-resolves the owner through Ring and resubscribes there,
//   - the node's pod watch must be synced; before that the subscription is
//     refused with "retry",
//   - the stream is the node's pods: a burst of their current state, then
//     changes; it closes when this replica loses the node (the consumer
//     resubscribes) or when the consumer cancels.
//
// Services and other cluster-scoped metadata are not streamed in v1; only
// pods. The bundle-backed service stream is a later increment.
func (s *LocalStore) Subscribe(ctx context.Context, node string, scope cm.Scope) (<-chan cm.NodeEvent, func(), error) {
	state := s.ring.State()
	if !contains(state.MyNodes, node) {
		return nil, nil, fmt.Errorf("this replica does not own node %s", node)
	}
	if !state.NodeSynced[node] {
		return nil, nil, fmt.Errorf("node %s is not synced yet, retry", node)
	}

	bundles := s.wmeta.Subscribe(nodeStreamName(node), workloadmeta.NormalPriority, nil)
	events := make(chan cm.NodeEvent, streamBufferSize)
	done := make(chan struct{})
	var closeDone sync.Once
	endStream := func() { closeDone.Do(func() { close(done) }) }

	unsubscribeOwned := s.ring.SubscribeOwnedNodes(func(prev, next []string) {
		if !contains(next, node) {
			// Ownership moved: the stream ends and the consumer resubscribes
			// to the new owner.
			endStream()
		}
	})

	pump := func() {
		defer close(events)
		defer s.wmeta.Unsubscribe(bundles)
		defer unsubscribeOwned()

		// podRef remembers each pod's location so unset events, which carry
		// only the entity ID, can still be routed and named.
		podsOnNode := make(map[string]*workloadmeta.KubernetesPod)

		for _, pod := range s.wmeta.ListKubernetesPods() {
			if pod.NodeName != node {
				continue
			}
			podsOnNode[pod.EntityID.ID] = pod
			if !s.sendPodEvent(events, done, pod, scope, false) {
				return
			}
		}

		for {
			select {
			case <-done:
				return
			case bundle, ok := <-bundles:
				if !ok {
					return
				}
				for _, event := range bundle.Events {
					pod, isPod := event.Entity.(*workloadmeta.KubernetesPod)
					if !isPod {
						continue
					}
					uid := pod.EntityID.ID
					switch event.Type {
					case workloadmeta.EventTypeSet:
						if pod.NodeName != node {
							continue
						}
						podsOnNode[uid] = pod
						if !s.sendPodEvent(events, done, pod, scope, false) {
							return
						}
					case workloadmeta.EventTypeUnset:
						known, watched := podsOnNode[uid]
						if !watched {
							continue
						}
						delete(podsOnNode, uid)
						if !s.sendPodEvent(events, done, known, scope, true) {
							return
						}
					}
				}
				close(bundle.Ch)
			}
		}
	}

	go pump()
	return events, endStream, nil
}

func nodeStreamName(node string) string {
	return "cluster-metadata-stream-" + node
}

// sendPodEvent emits one NodeEvent for a pod. Deleted events arrive with a
// bare entity, so the caller passes the last known pod for its namespace and
// name. It returns false once the stream is done.
func (s *LocalStore) sendPodEvent(events chan<- cm.NodeEvent, done <-chan struct{}, pod *workloadmeta.KubernetesPod, scope cm.Scope, deleted bool) bool {
	event := cm.NodeEvent{
		Kind:      KindPod,
		Namespace: pod.Namespace,
		Name:      pod.Name,
		Deleted:   deleted,
	}
	if !deleted {
		answer, err := s.tagAnswer(taggertypes.NewEntityID(taggertypes.KubernetesPodUID, pod.EntityID.ID), scope.Cardinality)
		if err != nil {
			return true
		}
		event.Tags = answer.Tags
	}

	select {
	case events <- event:
		return true
	case <-done:
		return false
	}
}
