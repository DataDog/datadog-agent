// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubeapiserver

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

// fakeScope is a controllable PodWatchScope: its node set changes on demand.
type fakeScope struct {
	nodes  []string
	notify func()
}

func (s *fakeScope) Nodes() []string { return s.nodes }

func (s *fakeScope) Subscribe(fn func()) func() {
	s.notify = fn
	fn()
	return func() { s.notify = nil }
}

// fakeReporter records sync transitions per node.
type fakeReporter struct {
	synced map[string]bool
}

func (r *fakeReporter) NodeSynced(node string, synced bool) {
	if r.synced == nil {
		r.synced = map[string]bool{}
	}
	r.synced[node] = synced
}

func nodePod(node, name string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
			UID:       types.UID(fmt.Sprintf("uid-%s-%s", node, name)),
		},
		Spec: corev1.PodSpec{NodeName: node},
	}
}

// fakeClientWithFieldSelectors works around the fake clientset ignoring
// field selectors on List: this reactor enforces spec.nodeName, matching
// the real API server behavior the watcher relies on.
func fakeClientWithFieldSelectors(pods ...*corev1.Pod) *k8sfake.Clientset {
	client := k8sfake.NewSimpleClientset()
	client.PrependReactor("list", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		listAction, ok := action.(k8stesting.ListActionImpl)
		if !ok {
			return false, nil, nil
		}
		node := strings.TrimPrefix(listAction.ListRestrictions.Fields.String(), "spec.nodeName=")
		if node == listAction.ListRestrictions.Fields.String() {
			// not a spec.nodeName selector: default behavior
			return false, nil, nil
		}

		list := &corev1.PodList{}
		for _, pod := range pods {
			if pod.Spec.NodeName == node {
				list.Items = append(list.Items, *pod)
			}
		}
		return true, list, nil
	})
	return client
}

// TestPerNodePodWatcher tests the scoped pod watch lifecycle.
// Test partitions:
// - scope nodes: one | several | changed (rebalance) | emptied
// - observed state: only scoped nodes' pods in the store
// - sync report: true on first list | false on stop
// - flush: lost node's pods unset from the store
func TestPerNodePodWatcher(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := fakeClientWithFieldSelectors(
		nodePod("node-a", "web-a1"),
		nodePod("node-a", "web-a2"),
		nodePod("node-b", "web-b1"),
	)
	scope := &fakeScope{nodes: []string{"node-a"}}
	reporter := &fakeReporter{}

	wmeta := mockedWorkloadmeta(t)
	watcher := newPerNodePodWatcher(wmeta, wmeta.GetConfig(), client, scope, reporter)
	go watcher.run(ctx)

	require.Eventually(t, func() bool {
		_, err := wmeta.GetKubernetesPod("uid-node-a-web-a1")
		return err == nil
	}, timeout, interval, "node-a pods are watched")
	require.Eventually(t, func() bool {
		return reporter.synced["node-a"]
	}, timeout, interval, "node-a sync reported")

	_, err := wmeta.GetKubernetesPod("uid-node-b-web-b1")
	assert.Error(t, err, "node-b is out of scope: its pods are not watched")

	// Rebalance: lose node-a, gain node-b.
	scope.nodes = []string{"node-b"}
	scope.notify()

	require.Eventually(t, func() bool {
		_, err := wmeta.GetKubernetesPod("uid-node-b-web-b1")
		return err == nil
	}, timeout, interval, "node-b pods are watched after rebalance")
	require.Eventually(t, func() bool {
		_, err := wmeta.GetKubernetesPod("uid-node-a-web-a1")
		return err != nil
	}, timeout, interval, "node-a pods flushed from the store after rebalance")
	assert.False(t, reporter.synced["node-a"], "node-a sync reported false on stop")

	// Empty scope: everything flushed.
	scope.nodes = nil
	scope.notify()
	require.Eventually(t, func() bool {
		_, err := wmeta.GetKubernetesPod("uid-node-b-web-b1")
		return err != nil
	}, timeout, interval, "all pods flushed when the scope empties")
}
