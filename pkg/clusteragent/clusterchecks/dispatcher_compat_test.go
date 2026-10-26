// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"fmt"
	"maps"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

// kubeGroups declares the "kube" runner group claiming the kube-family checks.
const kubeGroups = `{"kube":["kubernetes_state_core","orchestrator"]}`

// newGroupDispatcher returns a dispatcher with the given runner groups
// declared (cluster_checks.runner_groups).
func newGroupDispatcher(t *testing.T, groups string) *dispatcher {
	t.Helper()
	configmock.New(t).SetInTest("cluster_checks.runner_groups", groups)
	return newDispatcher(taggerfxmock.SetupFakeTagger(t))
}

// registerWorker registers a worker in the given runner group ("" for a
// general worker) via the status POST path, the way real workers do.
func registerWorker(t *testing.T, d *dispatcher, name, ip string, nodeType types.NodeType, group string) {
	t.Helper()
	d.processNodeStatus(name, ip, types.NodeStatus{NodeType: nodeType, Group: group})
}

func TestParseRunnerGroups(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		wantChecks map[string]string
		wantGroups []string
	}{
		{"not set", "", nil, nil},
		{"invalid JSON is ignored", "not-json", nil, nil},
		{"groups", `{"kube":["kubernetes_state_core","orchestrator"],"kafka":["kafka_consumer"]}`,
			map[string]string{"kubernetes_state_core": "kube", "orchestrator": "kube", "kafka_consumer": "kafka"}, []string{"kafka", "kube"}},
		{"a check claimed twice stays with the first group in name order", `{"b":["x"],"a":["x","y"]}`,
			map[string]string{"x": "a", "y": "a"}, []string{"a", "b"}},
		{"unnamed group is ignored", `{"":["x"],"kube":["y"]}`, map[string]string{"y": "kube"}, []string{"kube"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checkGroup, runnerGroups := parseRunnerGroups(tt.raw)
			assert.Equal(t, tt.wantChecks, checkGroup)
			assert.ElementsMatch(t, tt.wantGroups, slices.Collect(maps.Keys(runnerGroups)))
		})
	}
}

func TestDispatchRespectsRunnerGroups(t *testing.T) {
	tests := []struct {
		name    string
		workers map[string]string // worker name -> group
		check   string
		want    []string
	}{
		{"claimed check only goes to its group", map[string]string{"kube-1": "kube", "agent-1": ""}, "kubernetes_state_core", []string{"kube-1"}},
		{"unclaimed check only goes to general workers", map[string]string{"kube-1": "kube", "agent-1": ""}, "http_check", []string{"agent-1"}},
		{"group down: claimed check has no worker (strict)", map[string]string{"agent-1": ""}, "kubernetes_state_core", nil},
		{"worker in an unknown group runs nothing", map[string]string{"rogue": "foo"}, "http_check", nil},
		{"several group workers", map[string]string{"kube-2": "kube", "kube-1": "kube", "agent-1": ""}, "orchestrator", []string{"kube-1", "kube-2"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newGroupDispatcher(t, kubeGroups)
			i := 0
			for name, group := range tt.workers {
				i++
				registerWorker(t, d, name, fmt.Sprintf("10.0.0.%d", i), types.NodeTypeCLCRunner, group)
			}

			d.store.RLock()
			assert.Equal(t, tt.want, d.groupNodes(d.checkGroup[tt.check]))
			d.store.RUnlock()

			node, anyNode := d.getNodeToScheduleCheck(tt.check)
			assert.True(t, anyNode)
			if len(tt.want) == 0 {
				assert.Empty(t, node)
			} else {
				assert.Contains(t, tt.want, node)
			}
			requireNotLocked(t, d.store)
		})
	}
}

func TestAddWithNoGroupWorkerDangles(t *testing.T) {
	d := newGroupDispatcher(t, kubeGroups)
	registerWorker(t, d, "agent-1", "10.0.0.1", types.NodeTypeNodeAgent, "")

	// The kube group has no live worker: the claimed config dangles instead
	// of falling back to the general worker.
	assert.False(t, d.add(generateIntegration("kubernetes_state_core")))
	d.store.RLock()
	assert.Len(t, d.store.danglingConfigs, 1)
	d.store.RUnlock()

	// Once a kube worker registers, the dangling config is dispatched to it.
	registerWorker(t, d, "kube-1", "10.0.0.2", types.NodeTypeCLCRunner, "kube")
	dangling := d.retrieveDangling()
	require.Len(t, dangling, 1)
	assert.True(t, d.add(dangling[0]))
	d.store.RLock()
	assert.Equal(t, "kube-1", d.store.digestToNode[dangling[0].Digest()])
	d.store.RUnlock()

	requireNotLocked(t, d.store)
}
