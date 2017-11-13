// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package kubernetes

import (
	"testing"

	payload "github.com/DataDog/agent-payload/gogen"
	"github.com/ericchiang/k8s/api/v1"
	metav1 "github.com/ericchiang/k8s/apis/meta/v1"
	"github.com/stretchr/testify/assert"
)

func ps(s string) *string {
	return &s
}

func TestParsePods(t *testing.T) {
	inPods := []*v1.Pod{
		{
			Metadata: &metav1.ObjectMeta{
				Name:      ps("test-pod-1"),
				Namespace: ps("default"),
				Labels: map[string]string{
					"test": "abcd",
					"role": "intake",
				},
				OwnerReferences: []*metav1.OwnerReference{{
					Kind: ps("ReplicationController"),
					Name: ps("kubernetes-dashboard"),
				}},
			},
			Status: &v1.PodStatus{
				ContainerStatuses: []*v1.ContainerStatus{
					{
						ContainerID: ps("e468b96ca4fcc9687cc3"),
						Image:       ps("datadog/docker-dd-agent"),
						ImageID:     ps("docker://sha256:7c4034e4"),
					},
					{
						ContainerID: ps("3dbeb56a5545f17c1af2"),
						Image:       ps("redis"),
						ImageID:     ps("docker://sha256:7c4034e4"),
					},
				},
			},
		},
		{
			Metadata: &metav1.ObjectMeta{
				Name:      ps("test-pod-2"),
				Namespace: ps("default"),
				Labels: map[string]string{
					"role":    "web",
					"another": "label",
					"bim":     "bop",
				},
				OwnerReferences: []*metav1.OwnerReference{{
					Kind: ps("ReplicaSet"),
					Name: ps("kube-dns-196007617"),
				}},
			},
			Status: &v1.PodStatus{
				ContainerStatuses: []*v1.ContainerStatus{
					{
						ContainerID: ps("ce749d07a5645291f2f"),
						Image:       ps("dd/web-app"),
						ImageID:     ps("docker://sha256:7c4034e4"),
					},
					{
						ContainerID: ps("57bb7bcd0c5b1e58daa0"),
						Image:       ps("dd/redis-cache"),
						ImageID:     ps("docker://sha256:7c4034e4"),
					},
				},
			},
		},
	}

	// Simple parsing with no services
	pods, _ := parsePods(inPods, []*payload.KubeMetadataPayload_Service{})
	assert := assert.New(t)
	for i, p := range pods {
		assert.Equal(inPods[i].Metadata.GetName(), p.Name)
		assert.Equal(inPods[i].Metadata.GetNamespace(), p.Namespace)
		assert.Equal(inPods[i].Metadata.GetLabels(), p.Labels)
		assert.Len(inPods[i].Status.ContainerStatuses, len(p.ContainerIds))
		assert.Empty(p.ServiceUids)
	}

	assert.Equal(pods[0].ReplicationController, "kubernetes-dashboard")
	assert.Equal(pods[1].ReplicaSet, "kube-dns-196007617")
}

func TestSetPodCreator(t *testing.T) {
	for _, tc := range []struct {
		ownerRefs   []*metav1.OwnerReference
		annotations map[string]string
		expected    *payload.KubeMetadataPayload_Pod
	}{
		{
			// No refs
			ownerRefs: []*metav1.OwnerReference{},
			expected:  &payload.KubeMetadataPayload_Pod{},
		},
		{
			ownerRefs: []*metav1.OwnerReference{{
				Kind: ps("ReplicationController"),
				Name: ps("kubernetes-dashboard"),
			}},
			expected: &payload.KubeMetadataPayload_Pod{
				ReplicationController: "kubernetes-dashboard",
			},
		},
		{
			ownerRefs: []*metav1.OwnerReference{{
				Kind: ps("ReplicaSet"),
				Name: ps("apptastic-app"),
			}},
			expected: &payload.KubeMetadataPayload_Pod{
				ReplicaSet: "apptastic-app",
			},
		},
		{
			ownerRefs: []*metav1.OwnerReference{{
				Kind: ps("Job"),
				Name: ps("hello"),
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
		{
			Namespace: "dd",
			Name:      "intake",
			Uid:       "intake",
			Selector: map[string]string{
				"role": "intake",
			},
		},
		{
			Namespace: "default",
			Name:      "gce-web",
			Uid:       "gce-web",
			Selector: map[string]string{
				"class":    "web",
				"provider": "gce",
			},
		},
		{
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
