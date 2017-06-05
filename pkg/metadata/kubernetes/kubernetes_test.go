package kubernetes

import (
	"testing"

	payload "github.com/DataDog/agent-payload/go"
	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/pkg/api/v1"
)

func TestParsePods(t *testing.T) {
	inPods := []v1.Pod{
		v1.Pod{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-pod-1",
				Namespace: "default",
				Labels: map[string]string{
					"test": "abcd",
					"role": "intake",
				},
				OwnerReferences: []v1.OwnerReference{v1.OwnerReference{
					Kind: "ReplicationController",
					Name: "kubernetes-dashboard",
				}},
			},
			Status: v1.PodStatus{
				ContainerStatuses: []v1.ContainerStatus{
					v1.ContainerStatus{
						ContainerID: "e468b96ca4fcc9687cc3",
						Image:       "datadog/docker-dd-agent",
						ImageID:     "docker://sha256:7c4034e4",
					},
					v1.ContainerStatus{
						ContainerID: "3dbeb56a5545f17c1af2",
						Image:       "redis",
						ImageID:     "docker://sha256:7c4034e4",
					},
				},
			},
		},
		v1.Pod{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-pod-2",
				Namespace: "default",
				Labels: map[string]string{
					"role":    "web",
					"another": "label",
					"bim":     "bop",
				},
				OwnerReferences: []v1.OwnerReference{v1.OwnerReference{
					Kind: "ReplicaSet",
					Name: "kube-dns-196007617",
				}},
			},
			Status: v1.PodStatus{
				ContainerStatuses: []v1.ContainerStatus{
					v1.ContainerStatus{
						ContainerID: "ce749d07a5645291f2f",
						Image:       "dd/web-app",
						ImageID:     "docker://sha256:7c4034e4",
					},
					v1.ContainerStatus{
						ContainerID: "57bb7bcd0c5b1e58daa0",
						Image:       "dd/redis-cache",
						ImageID:     "docker://sha256:7c4034e4",
					},
				},
			},
		},
	}

	// Simple parsing with no services
	pods, _ := parsePods(inPods, []*payload.KubeMetadataPayload_Service{})
	assert := assert.New(t)
	for i, p := range pods {
		assert.Equal(inPods[i].Name, p.Name)
		assert.Equal(inPods[i].Namespace, p.Namespace)
		assert.Equal(inPods[i].Labels, p.Labels)
		assert.Len(inPods[i].Status.ContainerStatuses, len(p.ContainerIds))
		assert.Empty(p.ServiceUids)
	}

	assert.Equal(pods[0].ReplicationController, "kubernetes-dashboard")
	assert.Equal(pods[1].ReplicaSet, "kube-dns-196007617")
}

func TestSetPodCreator(t *testing.T) {
	for _, tc := range []struct {
		ownerRefs   []v1.OwnerReference
		annotations map[string]string
		expected    *payload.KubeMetadataPayload_Pod
	}{
		{
			// No refs
			ownerRefs: []v1.OwnerReference{},
			expected:  &payload.KubeMetadataPayload_Pod{},
		},
		{
			ownerRefs: []v1.OwnerReference{v1.OwnerReference{
				Kind: "ReplicationController",
				Name: "kubernetes-dashboard",
			}},
			expected: &payload.KubeMetadataPayload_Pod{
				ReplicationController: "kubernetes-dashboard",
			},
		},
		{
			ownerRefs: []v1.OwnerReference{v1.OwnerReference{
				Kind: "ReplicaSet",
				Name: "apptastic-app",
			}},
			expected: &payload.KubeMetadataPayload_Pod{
				ReplicaSet: "apptastic-app",
			},
		},
		{
			ownerRefs: []v1.OwnerReference{v1.OwnerReference{
				Kind: "Job",
				Name: "hello",
			}},
			expected: &payload.KubeMetadataPayload_Pod{
				Job: "hello",
			},
		},
	} {
		pod := &payload.KubeMetadataPayload_Pod{}
		setPodCreator(pod, tc.ownerRefs)
		assert.Equal(t, tc.expected, pod)
	}
}

func TestFindPodServices(t *testing.T) {
	services := []*payload.KubeMetadataPayload_Service{
		&payload.KubeMetadataPayload_Service{
			Namespace: "dd",
			Name:      "intake",
			Uid:       "intake",
			Selector: map[string]string{
				"role": "intake",
			},
		},
		&payload.KubeMetadataPayload_Service{
			Namespace: "default",
			Name:      "gce-web",
			Uid:       "gce-web",
			Selector: map[string]string{
				"class":    "web",
				"provider": "gce",
			},
		},
		&payload.KubeMetadataPayload_Service{
			Namespace: "default",
			Name:      "aws-web",
			Uid:       "aws-web",
			Selector: map[string]string{
				"class":    "web",
				"provider": "aws",
			},
		},
	}

	for _, tc := range []struct {
		labels    map[string]string
		namespace string
		expected  []string
	}{
		{
			labels:    map[string]string{},
			namespace: "default",
			expected:  []string{},
		},
		{
			labels: map[string]string{
				"role":  "intake",
				"label": "with-something-else",
			},
			namespace: "dd",
			expected:  []string{"intake"},
		},
		{
			labels: map[string]string{
				"role": "intake",
			},
			namespace: "default",
			expected:  []string{},
		},
		{
			labels: map[string]string{
				"class":    "web",
				"provider": "aws",
			},
			namespace: "default",
			expected:  []string{"aws-web"},
		},
	} {
		uids := findPodServices(tc.namespace, tc.labels, services)
		assert.Equal(t, tc.expected, uids)
	}
}
