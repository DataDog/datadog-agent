// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubelet

package collectors

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

func TestParsePods(t *testing.T) {
	dockerEntityID := "docker://d0242fc32d53137526dc365e7c86ef43b5f50b6f72dfd53dcb948eff4560376f"
	dockerContainerStatus := kubelet.Status{
		Containers: []kubelet.ContainerStatus{
			{
				ID:    dockerEntityID,
				Image: "datadog/docker-dd-agent:latest5",
				Name:  "dd-agent",
			},
		},
	}
	dockerContainerSpec := kubelet.Spec{
		Containers: []kubelet.ContainerSpec{
			{
				Name:  "dd-agent",
				Image: "datadog/docker-dd-agent:latest5",
			},
		},
	}

	criEntityId := "cri-containerd://acbe44ff07525934cab9bf7c38c6627d64fd0952d8e6b87535d57092bfa6e9d1"
	criContainerStatus := kubelet.Status{
		Containers: []kubelet.ContainerStatus{
			{
				ID:    criEntityId,
				Image: "sha256:0f006d265944c984e05200fab1c14ac54163cbcd4e8ae0ba3b35eb46fc559823",
				Name:  "redis-master",
			},
		},
	}
	criContainerSpec := kubelet.Spec{
		Containers: []kubelet.ContainerSpec{
			{
				Name:  "redis-master",
				Image: "gcr.io/google_containers/redis:e2e",
			},
		},
	}

	for nb, tc := range []struct {
		desc              string
		pod               *kubelet.Pod
		labelsAsTags      map[string]string
		annotationsAsTags map[string]string
		expectedInfo      *TagInfo
	}{
		{
			desc:         "empty pod",
			pod:          &kubelet.Pod{},
			labelsAsTags: map[string]string{},
			expectedInfo: nil,
		},
		{
			desc: "daemonset + common tags",
			pod: &kubelet.Pod{
				Metadata: kubelet.PodMetadata{
					Name:      "dd-agent-rc-qd876",
					Namespace: "default",
					Owners: []kubelet.PodOwner{
						{
							Kind: "DaemonSet",
							Name: "dd-agent-rc",
							ID:   "6a76e51c-88d7-11e7-9a0f-42010a8401cc",
						},
					},
				},
				Status: dockerContainerStatus,
				Spec:   dockerContainerSpec,
			},
			labelsAsTags: map[string]string{},
			expectedInfo: &TagInfo{
				Source: "kubelet",
				Entity: dockerEntityID,
				LowCardTags: []string{
					"kube_namespace:default",
					"kube_container_name:dd-agent",
					"kube_daemon_set:dd-agent-rc",
					"image_tag:latest5",
					"image_name:datadog/docker-dd-agent",
					"short_image:docker-dd-agent",
				},
				HighCardTags: []string{"pod_name:dd-agent-rc-qd876"},
			},
		},
		{
			desc: "standalone replicaset",
			pod: &kubelet.Pod{
				Metadata: kubelet.PodMetadata{
					Owners: []kubelet.PodOwner{
						{
							Kind: "ReplicaSet",
							Name: "kubernetes-dashboard",
						},
					},
				},
				Status: dockerContainerStatus,
				Spec:   dockerContainerSpec,
			},
			labelsAsTags: map[string]string{},
			expectedInfo: &TagInfo{
				Source: "kubelet",
				Entity: dockerEntityID,
				LowCardTags: []string{
					"kube_container_name:dd-agent",
					"kube_replica_set:kubernetes-dashboard",
					"image_tag:latest5",
					"image_name:datadog/docker-dd-agent",
					"short_image:docker-dd-agent",
				},
				HighCardTags: []string{},
			},
		},
		{
			desc: "replicaset to daemonset < 1.8",
			pod: &kubelet.Pod{
				Metadata: kubelet.PodMetadata{
					Owners: []kubelet.PodOwner{
						{
							Kind: "ReplicaSet",
							Name: "frontend-2891696001",
						},
					},
				},
				Status: dockerContainerStatus,
				Spec:   dockerContainerSpec,
			},
			labelsAsTags: map[string]string{},
			expectedInfo: &TagInfo{
				Source: "kubelet",
				Entity: dockerEntityID,
				LowCardTags: []string{
					"kube_container_name:dd-agent",
					"kube_deployment:frontend",
					"image_tag:latest5",
					"image_name:datadog/docker-dd-agent",
					"short_image:docker-dd-agent",
				},
				HighCardTags: []string{"kube_replica_set:frontend-2891696001"},
			},
		},
		{
			desc: "replicaset to daemonset 1.8+",
			pod: &kubelet.Pod{
				Metadata: kubelet.PodMetadata{
					Owners: []kubelet.PodOwner{
						{
							Kind: "ReplicaSet",
							Name: "front-end-768dd754b7",
						},
					},
				},
				Status: dockerContainerStatus,
				Spec:   dockerContainerSpec,
			},
			labelsAsTags: map[string]string{},
			expectedInfo: &TagInfo{
				Source: "kubelet",
				Entity: dockerEntityID,
				LowCardTags: []string{
					"kube_container_name:dd-agent",
					"kube_deployment:front-end",
					"image_tag:latest5",
					"image_name:datadog/docker-dd-agent",
					"short_image:docker-dd-agent",
				},
				HighCardTags: []string{"kube_replica_set:front-end-768dd754b7"},
			},
		},
		{
			desc: "pod labels",
			pod: &kubelet.Pod{
				Metadata: kubelet.PodMetadata{
					Labels: map[string]string{
						"component":         "kube-proxy",
						"tier":              "node",
						"k8s-app":           "kubernetes-dashboard",
						"pod-template-hash": "490794276",
						"GitCommit":         "ea38b55f07e40b68177111a2bff1e918132fd5fb",
						"OwnerTeam":         "Kenafeh",
					},
				},
				Status: dockerContainerStatus,
				Spec:   dockerContainerSpec,
			},
			labelsAsTags: map[string]string{
				"component": "component",
				"ownerteam": "team",
				"gitcommit": "+GitCommit",
				"tier":      "tier",
			},
			expectedInfo: &TagInfo{
				Source: "kubelet",
				Entity: dockerEntityID,
				LowCardTags: []string{
					"kube_container_name:dd-agent",
					"team:Kenafeh",
					"component:kube-proxy",
					"tier:node",
					"image_tag:latest5",
					"image_name:datadog/docker-dd-agent",
					"short_image:docker-dd-agent",
				},
				HighCardTags: []string{"GitCommit:ea38b55f07e40b68177111a2bff1e918132fd5fb"},
			},
		},
		{
			desc: "pod labels + annotations",
			pod: &kubelet.Pod{
				Metadata: kubelet.PodMetadata{
					Labels: map[string]string{
						"component":         "kube-proxy",
						"tier":              "node",
						"k8s-app":           "kubernetes-dashboard",
						"pod-template-hash": "490794276",
					},
					Annotations: map[string]string{
						"noTag":     "don't collect",
						"GitCommit": "ea38b55f07e40b68177111a2bff1e918132fd5fb",
						"OwnerTeam": "Kenafeh",
					},
				},
				Status: dockerContainerStatus,
				Spec:   dockerContainerSpec,
			},
			labelsAsTags: map[string]string{
				"component": "component",
				"tier":      "tier",
			},
			annotationsAsTags: map[string]string{
				"ownerteam": "team",
				"gitcommit": "+GitCommit",
			},
			expectedInfo: &TagInfo{
				Source: "kubelet",
				Entity: dockerEntityID,
				LowCardTags: []string{
					"kube_container_name:dd-agent",
					"team:Kenafeh",
					"component:kube-proxy",
					"tier:node",
					"image_tag:latest5",
					"image_name:datadog/docker-dd-agent",
					"short_image:docker-dd-agent",
				},
				HighCardTags: []string{"GitCommit:ea38b55f07e40b68177111a2bff1e918132fd5fb"},
			},
		},
		{
			desc: "openshift deploymentconfig",
			pod: &kubelet.Pod{
				Metadata: kubelet.PodMetadata{
					Annotations: map[string]string{
						"openshift.io/deployment-config.latest-version": "1",
						"openshift.io/deployment-config.name":           "gitlab-ce",
						"openshift.io/deployment.name":                  "gitlab-ce-1",
					},
				},
				Status: dockerContainerStatus,
			},
			labelsAsTags: map[string]string{},
			expectedInfo: &TagInfo{
				Source:       "kubelet",
				Entity:       dockerEntityID,
				LowCardTags:  []string{"kube_container_name:dd-agent", "oshift_deployment_config:gitlab-ce"},
				HighCardTags: []string{"oshift_deployment:gitlab-ce-1"},
			},
		},
		{
			desc: "CRI pod",
			pod: &kubelet.Pod{
				Metadata: kubelet.PodMetadata{
					Owners: []kubelet.PodOwner{
						{
							Kind: "ReplicaSet",
							Name: "redis-master-546dc4865f",
						},
					},
				},
				Status: criContainerStatus,
				Spec:   criContainerSpec,
			},
			labelsAsTags: map[string]string{},
			expectedInfo: &TagInfo{
				Source: "kubelet",
				Entity: criEntityId,
				LowCardTags: []string{
					"kube_container_name:redis-master",
					"kube_deployment:redis-master",
					"image_name:gcr.io/google_containers/redis",
					"image_tag:e2e",
					"short_image:redis",
				},
				HighCardTags: []string{"kube_replica_set:redis-master-546dc4865f"},
			},
		},
	} {
		t.Run(fmt.Sprintf("case %d: %s", nb, tc.desc), func(t *testing.T) {
			collector := &KubeletCollector{
				labelsAsTags:      tc.labelsAsTags,
				annotationsAsTags: tc.annotationsAsTags,
			}
			infos, err := collector.parsePods([]*kubelet.Pod{tc.pod})
			assert.Nil(t, err)

			if tc.expectedInfo == nil {
				assert.Len(t, infos, 0)
			} else {
				assert.Len(t, infos, 1)
				assertTagInfoEqual(t, tc.expectedInfo, infos[0])
			}
		})
	}
}

func TestParseDeploymentForReplicaset(t *testing.T) {
	for in, out := range map[string]string{
		// Nominal 1.6 cases
		"frontend-2891696001":  "frontend",
		"front-end-2891696001": "front-end",

		// Non-deployment 1.6 cases
		"frontend2891696001":  "",
		"-frontend2891696001": "",
		"manually-created":    "",

		// 1.8+ nominal cases
		"frontend-56c89cfff7":   "frontend",
		"frontend-56c":          "frontend",
		"frontend-56c89cff":     "frontend",
		"frontend-56c89cfff7c2": "frontend",
		"front-end-768dd754b7":  "front-end",

		// 1.8+ non-deployment cases
		"frontend-5f":         "", // too short
		"frontend-56a89cfff7": "", // no vowels allowed
	} {
		t.Run(fmt.Sprintf("case: %s", in), func(t *testing.T) {
			collector := &KubeletCollector{}
			assert.Equal(t, out, collector.parseDeploymentForReplicaset(in))
		})
	}
}
