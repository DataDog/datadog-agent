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
	entityID := "docker://d0242fc32d53137526dc365e7c86ef43b5f50b6f72dfd53dcb948eff4560376f"
	oneContainer := kubelet.Status{
		Containers: []kubelet.ContainerStatus{
			{
				ID:    entityID,
				Image: "datadog/docker-dd-agent:latest5",
				Name:  "dd-agent",
			},
		},
	}

	for nb, tc := range []struct {
		desc         string
		pod          *kubelet.Pod
		labelsAsTags map[string]string
		expectedInfo *TagInfo
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
				Status: oneContainer,
			},
			labelsAsTags: map[string]string{},
			expectedInfo: &TagInfo{
				Source:       "kubelet",
				Entity:       entityID,
				LowCardTags:  []string{"kube_namespace:default", "kube_container_name:dd-agent", "kube_daemon_set:dd-agent-rc"},
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
				Status: oneContainer,
			},
			labelsAsTags: map[string]string{},
			expectedInfo: &TagInfo{
				Source:       "kubelet",
				Entity:       entityID,
				LowCardTags:  []string{"kube_container_name:dd-agent", "kube_replica_set:kubernetes-dashboard"},
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
				Status: oneContainer,
			},
			labelsAsTags: map[string]string{},
			expectedInfo: &TagInfo{
				Source:       "kubelet",
				Entity:       entityID,
				LowCardTags:  []string{"kube_container_name:dd-agent", "kube_deployment:frontend"},
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
				Status: oneContainer,
			},
			labelsAsTags: map[string]string{},
			expectedInfo: &TagInfo{
				Source:       "kubelet",
				Entity:       entityID,
				LowCardTags:  []string{"kube_container_name:dd-agent", "kube_deployment:front-end"},
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
				Status: oneContainer,
			},
			labelsAsTags: map[string]string{
				"component": "component",
				"ownerteam": "team",
				"gitcommit": "+GitCommit",
				"tier":      "tier",
			},
			expectedInfo: &TagInfo{
				Source: "kubelet",
				Entity: entityID,
				LowCardTags: []string{
					"kube_container_name:dd-agent",
					"team:Kenafeh",
					"component:kube-proxy",
					"tier:node",
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
				Status: oneContainer,
			},
			labelsAsTags: map[string]string{},
			expectedInfo: &TagInfo{
				Source:       "kubelet",
				Entity:       entityID,
				LowCardTags:  []string{"kube_container_name:dd-agent", "oshift_deployment_config:gitlab-ce"},
				HighCardTags: []string{"oshift_deployment:gitlab-ce-1"},
			},
		},
	} {
		t.Run(fmt.Sprintf("case %d: %s", nb, tc.desc), func(t *testing.T) {
			collector := &KubeletCollector{
				labelsAsTags: tc.labelsAsTags,
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
