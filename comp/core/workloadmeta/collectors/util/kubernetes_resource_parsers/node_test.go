// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubernetesresourceparsers

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

func parseNode(t *testing.T, node *corev1.Node) *workloadmeta.KubernetesNode {
	t.Helper()
	entity := NewNodeParser().Parse(node)
	n, ok := entity.(*workloadmeta.KubernetesNode)
	require.True(t, ok)
	return n
}

func TestNodeParserConditionsAndCordon(t *testing.T) {
	now := metav1.NewTime(time.Now())
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
		Spec: corev1.NodeSpec{
			Unschedulable: true,
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionFalse, LastHeartbeatTime: now, LastTransitionTime: now, Reason: "KubeletNotReady", Message: "boom"},
				{Type: "nodehold", Status: corev1.ConditionTrue},
			},
		},
	}

	n := parseNode(t, node)

	assert.Equal(t, []workloadmeta.KubernetesNodeCondition{
		{Type: "Ready", Status: "False"},
		{Type: "nodehold", Status: "True"},
	}, n.Conditions)
	assert.True(t, n.Unschedulable)
}

func TestNodeParserWithoutConditions(t *testing.T) {
	n := parseNode(t, &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-1"}})

	assert.Empty(t, n.Conditions)
	assert.False(t, n.Unschedulable)
}

// The cluster agent serves Status as is on /api/v1/info/node; the conditions and
// cordon state are not part of that wire format.
func TestNodeParserStatusWireFormatUnchanged(t *testing.T) {
	n := parseNode(t, &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
		Spec:       corev1.NodeSpec{Unschedulable: true},
		Status:     corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}},
	})

	data, err := json.Marshal(&n.Status)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"kubeletVersion": "", "kernelVersion": "", "osImage": "", "containerRuntimeVersion": "",
		"architecture": "", "operatingSystem": ""
	}`, string(data))
}

func TestNodeDeepCopyKeepsConditions(t *testing.T) {
	n := parseNode(t, &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
		Spec:       corev1.NodeSpec{Unschedulable: true},
		Status:     corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: "nodehold", Status: corev1.ConditionTrue}}},
	})

	cp, ok := n.DeepCopy().(*workloadmeta.KubernetesNode)
	require.True(t, ok)
	assert.Equal(t, n, cp)
	cp.Conditions[0].Status = "False"
	assert.Equal(t, "True", n.Conditions[0].Status)
}
