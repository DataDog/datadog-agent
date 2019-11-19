// +build linux,kubelet

package checks

import (
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/process"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestExtractPodMessage(t *testing.T) {
	timestamp := metav1.NewTime(time.Date(2014, time.January, 15, 0, 0, 0, 0, time.UTC))
	pod := v1.Pod{
		Status: v1.PodStatus{
			Phase:             v1.PodRunning,
			StartTime:         &timestamp,
			NominatedNodeName: "nominated",
			Conditions: []v1.PodCondition{
				{
					Type:   v1.PodReady,
					Status: v1.ConditionTrue,
				},
			},
			ContainerStatuses: []v1.ContainerStatus{
				{
					Name:         "fooName",
					Image:        "fooImage",
					ContainerID:  "docker://fooID",
					RestartCount: 13,
					State: v1.ContainerState{
						Running: &v1.ContainerStateRunning{
							StartedAt: timestamp,
						},
					},
				},
				{
					Name:         "barName",
					Image:        "barImage",
					ContainerID:  "docker://barID",
					RestartCount: 10,
					State: v1.ContainerState{
						Waiting: &v1.ContainerStateWaiting{
							Reason:  "chillin",
							Message: "testin",
						},
					},
				},
				{
					Name:         "bazName",
					Image:        "bazImage",
					ContainerID:  "docker://bazID",
					RestartCount: 19,
					State: v1.ContainerState{
						Terminated: &v1.ContainerStateTerminated{
							ExitCode: -1,
							Signal:   9,
							Reason:   "CLB",
							Message:  "PLS",
						},
					},
				},
			},
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod",
			Namespace: "namespace",
			Labels: map[string]string{
				"label": "foo",
			},
			Annotations: map[string]string{
				"annotation": "bar",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					Name: "test-controller",
					Kind: "replicaset",
					UID:  types.UID("1234567890"),
				},
			},
		},
		Spec: v1.PodSpec{
			NodeName:   "node",
			Containers: []v1.Container{{}, {}},
		},
	}

	expectedModel := model.Pod{
		Name:              "pod",
		Namespace:         "namespace",
		CreationTimestamp: -62135596800,
		Phase:             "Running",
		NominatedNodeName: "nominated",
		NodeName:          "node",
		RestartCount:      42,
		Labels:            []string{"label:foo"},
		Annotations:       []string{"annotation:bar"},
		OwnerReferences: []*model.OwnerReference{
			{
				Name: "test-controller",
				Kind: "replicaset",
				Uid:  "1234567890",
			},
		},
		ContainerStatuses: []*model.ContainerStatus{
			{
				State:        "Running",
				RestartCount: 13,
				Name:         "fooName",
				ContainerID:  "docker://fooID",
			},
			{
				State:        "Waiting",
				Message:      "chillin testin",
				RestartCount: 10,
				Name:         "barName",
				ContainerID:  "docker://barID",
			},
			{
				State:        "Terminated",
				Message:      "CLB PLS (exit: -1)",
				RestartCount: 19,
				Name:         "bazName",
				ContainerID:  "docker://bazID",
			},
		},
	}

	assert.Equal(t, &expectedModel, extractPodMessage(&pod))
}
