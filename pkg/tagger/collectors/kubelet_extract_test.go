// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubelet

package collectors

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

func TestParsePods(t *testing.T) {
	dockerEntityID := "container_id://d0242fc32d53137526dc365e7c86ef43b5f50b6f72dfd53dcb948eff4560376f"
	dockerContainerStatus := kubelet.Status{
		Containers: []kubelet.ContainerStatus{
			{
				ID:    dockerEntityID,
				Image: "datadog/docker-dd-agent:latest5",
				Name:  "dd-agent",
			},
		},
		Phase: "Running",
	}
	dockerContainerSpec := kubelet.Spec{
		Containers: []kubelet.ContainerSpec{
			{
				Name:  "dd-agent",
				Image: "datadog/docker-dd-agent:latest5",
			},
		},
	}
	dockerContainerSpecWithEnv := kubelet.Spec{
		Containers: []kubelet.ContainerSpec{
			{
				Name:  "dd-agent",
				Image: "datadog/docker-dd-agent:latest5",
				Env: []kubelet.EnvVar{
					{
						Name:  "DD_ENV",
						Value: "production",
					},
					{
						Name:  "DD_SERVICE",
						Value: "dd-agent",
					},
					{
						Name:  "DD_VERSION",
						Value: "1.1.0",
					},
				},
			},
		},
	}

	dockerEntityID2 := "container_id://ff242fc32d53137526dc365e7c86ef43b5f50b6f72dfd53dcb948eff4560376f"
	dockerTwoContainersStatus := kubelet.Status{
		Containers: []kubelet.ContainerStatus{
			{
				ID:    dockerEntityID,
				Image: "datadog/docker-dd-agent:latest5",
				Name:  "dd-agent",
			},
			{
				ID:    dockerEntityID2,
				Image: "datadog/docker-filter:latest",
				Name:  "filter",
			},
		},
		Phase: "Pending",
	}
	dockerTwoContainersSpec := kubelet.Spec{
		Containers: []kubelet.ContainerSpec{
			{
				Name:  "dd-agent",
				Image: "datadog/docker-dd-agent:latest5",
			},
			{
				Name:  "filter",
				Image: "datadog/docker-filter:latest",
			},
		},
	}

	dockerEntityIDCassandra := "container_id://6eaa4782de428f5ea639e33a837ed47aa9fa9e6969f8cb23e39ff788a751ce7d"
	dockerContainerStatusCassandra := kubelet.Status{
		Containers: []kubelet.ContainerStatus{
			{
				ID:    dockerEntityIDCassandra,
				Image: "gcr.io/google-samples/cassandra:v13",
				Name:  "cassandra",
			},
		},
		Phase: "Running",
	}
	dockerContainerSpecCassandra := kubelet.Spec{
		Containers: []kubelet.ContainerSpec{
			{
				Name:  "cassandra",
				Image: "gcr.io/google-samples/cassandra:v13",
			},
		},
		Volumes: []kubelet.VolumeSpec{
			{
				Name: "cassandra-data",
				PersistentVolumeClaim: &kubelet.PersistentVolumeClaimSpec{
					ClaimName: "cassandra-data-cassandra-0",
				},
			},
		},
	}

	dockerContainerSpecCassandraMultiplePvcs := kubelet.Spec{
		Containers: []kubelet.ContainerSpec{
			{
				Name:  "cassandra",
				Image: "gcr.io/google-samples/cassandra:v13",
			},
		},
		Volumes: []kubelet.VolumeSpec{
			{
				Name: "cassandra-data",
				PersistentVolumeClaim: &kubelet.PersistentVolumeClaimSpec{
					ClaimName: "cassandra-data-cassandra-0",
				},
			},
			{
				Name: "another-pvc",
				PersistentVolumeClaim: &kubelet.PersistentVolumeClaimSpec{
					ClaimName: "another-pvc-data-0",
				},
			},
		},
	}

	criEntityID := "container_id://acbe44ff07525934cab9bf7c38c6627d64fd0952d8e6b87535d57092bfa6e9d1"
	criContainerStatus := kubelet.Status{
		Containers: []kubelet.ContainerStatus{
			{
				ID:    criEntityID,
				Image: "sha256:0f006d265944c984e05200fab1c14ac54163cbcd4e8ae0ba3b35eb46fc559823",
				Name:  "redis-master",
			},
		},
		Phase: "Running",
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
		skip              bool
		desc              string
		pod               *kubelet.Pod
		labelsAsTags      map[string]string
		annotationsAsTags map[string]string
		expectedInfo      []*TagInfo
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
			expectedInfo: []*TagInfo{{
				Source: "kubelet",
				Entity: dockerEntityID,
				LowCardTags: []string{
					"kube_namespace:default",
					"kube_container_name:dd-agent",
					"kube_daemon_set:dd-agent-rc",
					"image_tag:latest5",
					"image_name:datadog/docker-dd-agent",
					"short_image:docker-dd-agent",
					"pod_phase:running",
				},
				OrchestratorCardTags: []string{
					"pod_name:dd-agent-rc-qd876",
				},
				HighCardTags: []string{
					"container_id:d0242fc32d53137526dc365e7c86ef43b5f50b6f72dfd53dcb948eff4560376f",
					"display_container_name:dd-agent_dd-agent-rc-qd876",
				},
			}},
		},
		{
			desc: "two containers + pod",
			pod: &kubelet.Pod{
				Metadata: kubelet.PodMetadata{
					Name:      "dd-agent-rc-qd876",
					Namespace: "default",
					UID:       "5e8e05",
					Owners: []kubelet.PodOwner{
						{
							Kind: "DaemonSet",
							Name: "dd-agent-rc",
							ID:   "6a76e51c-88d7-11e7-9a0f-42010a8401cc",
						},
					},
				},
				Status: dockerTwoContainersStatus,
				Spec:   dockerTwoContainersSpec,
			},
			labelsAsTags: map[string]string{},
			expectedInfo: []*TagInfo{
				{
					Source: "kubelet",
					Entity: "kubernetes_pod_uid://5e8e05",
					LowCardTags: []string{
						"kube_namespace:default",
						"kube_daemon_set:dd-agent-rc",
						"pod_phase:pending",
					},
					OrchestratorCardTags: []string{
						"pod_name:dd-agent-rc-qd876",
					},
					HighCardTags: []string{},
				},
				{
					Source: "kubelet",
					Entity: dockerEntityID,
					LowCardTags: []string{
						"kube_namespace:default",
						"kube_container_name:dd-agent",
						"kube_daemon_set:dd-agent-rc",
						"image_tag:latest5",
						"image_name:datadog/docker-dd-agent",
						"short_image:docker-dd-agent",
						"pod_phase:pending",
					},
					OrchestratorCardTags: []string{
						"pod_name:dd-agent-rc-qd876",
					},
					HighCardTags: []string{
						"container_id:d0242fc32d53137526dc365e7c86ef43b5f50b6f72dfd53dcb948eff4560376f",
						"display_container_name:dd-agent_dd-agent-rc-qd876",
					},
				},
				{
					Source: "kubelet",
					Entity: dockerEntityID2,
					LowCardTags: []string{
						"kube_namespace:default",
						"kube_container_name:filter",
						"kube_daemon_set:dd-agent-rc",
						"image_tag:latest",
						"image_name:datadog/docker-filter",
						"short_image:docker-filter",
						"pod_phase:pending",
					},
					OrchestratorCardTags: []string{
						"pod_name:dd-agent-rc-qd876",
					},
					HighCardTags: []string{
						"container_id:ff242fc32d53137526dc365e7c86ef43b5f50b6f72dfd53dcb948eff4560376f",
						"display_container_name:filter_dd-agent-rc-qd876",
					},
				},
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
			expectedInfo: []*TagInfo{{
				Source: "kubelet",
				Entity: dockerEntityID,
				LowCardTags: []string{
					"kube_container_name:dd-agent",
					"kube_replica_set:kubernetes-dashboard",
					"image_tag:latest5",
					"image_name:datadog/docker-dd-agent",
					"short_image:docker-dd-agent",
					"pod_phase:running",
				},
				OrchestratorCardTags: []string{},
				HighCardTags: []string{
					"container_id:d0242fc32d53137526dc365e7c86ef43b5f50b6f72dfd53dcb948eff4560376f",
				},
			}},
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
			expectedInfo: []*TagInfo{{
				Source: "kubelet",
				Entity: dockerEntityID,
				LowCardTags: []string{
					"kube_container_name:dd-agent",
					"kube_deployment:frontend",
					"image_tag:latest5",
					"image_name:datadog/docker-dd-agent",
					"short_image:docker-dd-agent",
					"pod_phase:running",
				},
				OrchestratorCardTags: []string{
					"kube_replica_set:frontend-2891696001",
				},
				HighCardTags: []string{
					"container_id:d0242fc32d53137526dc365e7c86ef43b5f50b6f72dfd53dcb948eff4560376f",
				},
			}},
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
			expectedInfo: []*TagInfo{{
				Source: "kubelet",
				Entity: dockerEntityID,
				LowCardTags: []string{
					"kube_container_name:dd-agent",
					"kube_deployment:front-end",
					"image_tag:latest5",
					"image_name:datadog/docker-dd-agent",
					"short_image:docker-dd-agent",
					"pod_phase:running",
				},
				OrchestratorCardTags: []string{
					"kube_replica_set:front-end-768dd754b7",
				},
				HighCardTags: []string{
					"container_id:d0242fc32d53137526dc365e7c86ef43b5f50b6f72dfd53dcb948eff4560376f",
				},
			}},
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
			expectedInfo: []*TagInfo{{
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
					"pod_phase:running",
				},
				OrchestratorCardTags: []string{},
				HighCardTags: []string{
					"container_id:d0242fc32d53137526dc365e7c86ef43b5f50b6f72dfd53dcb948eff4560376f",
					"GitCommit:ea38b55f07e40b68177111a2bff1e918132fd5fb",
				},
			}},
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
						"noTag":                          "don't collect",
						"GitCommit":                      "ea38b55f07e40b68177111a2bff1e918132fd5fb",
						"OwnerTeam":                      "Kenafeh",
						"ad.datadoghq.com/tags":          `{"pod_template_version": "1.0.0"}`,
						"ad.datadoghq.com/dd-agent.tags": `{"agent_version": "6.9.0"}`,
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
			expectedInfo: []*TagInfo{{
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
					"pod_template_version:1.0.0",
					"agent_version:6.9.0",
					"pod_phase:running",
				},
				OrchestratorCardTags: []string{},
				HighCardTags: []string{
					"container_id:d0242fc32d53137526dc365e7c86ef43b5f50b6f72dfd53dcb948eff4560376f",
					"GitCommit:ea38b55f07e40b68177111a2bff1e918132fd5fb",
				},
			}},
		},
		{
			desc: "standard pod labels",
			pod: &kubelet.Pod{
				Metadata: kubelet.PodMetadata{
					Labels: map[string]string{
						"component":                  "kube-proxy",
						"tier":                       "node",
						"k8s-app":                    "kubernetes-dashboard",
						"pod-template-hash":          "490794276",
						"tags.datadoghq.com/env":     "production",
						"tags.datadoghq.com/service": "dd-agent",
						"tags.datadoghq.com/version": "1.1.0",
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
			expectedInfo: []*TagInfo{{
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
					"env:production",
					"service:dd-agent",
					"version:1.1.0",
					"pod_phase:running",
				},
				OrchestratorCardTags: []string{},
				HighCardTags: []string{
					"container_id:d0242fc32d53137526dc365e7c86ef43b5f50b6f72dfd53dcb948eff4560376f",
					"GitCommit:ea38b55f07e40b68177111a2bff1e918132fd5fb",
				},
			}},
		},
		{
			desc: "standard container labels",
			pod: &kubelet.Pod{
				Metadata: kubelet.PodMetadata{
					Labels: map[string]string{
						"component":                           "kube-proxy",
						"tier":                                "node",
						"k8s-app":                             "kubernetes-dashboard",
						"pod-template-hash":                   "490794276",
						"tags.datadoghq.com/dd-agent.env":     "production",
						"tags.datadoghq.com/dd-agent.service": "dd-agent",
						"tags.datadoghq.com/dd-agent.version": "1.1.0",
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
			expectedInfo: []*TagInfo{{
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
					"env:production",
					"service:dd-agent",
					"version:1.1.0",
					"pod_phase:running",
				},
				OrchestratorCardTags: []string{},
				HighCardTags: []string{
					"container_id:d0242fc32d53137526dc365e7c86ef43b5f50b6f72dfd53dcb948eff4560376f",
					"GitCommit:ea38b55f07e40b68177111a2bff1e918132fd5fb",
				},
			}},
		},
		{
			desc: "standard pod + container labels",
			pod: &kubelet.Pod{
				Metadata: kubelet.PodMetadata{
					Labels: map[string]string{
						"component":                           "kube-proxy",
						"tier":                                "node",
						"k8s-app":                             "kubernetes-dashboard",
						"pod-template-hash":                   "490794276",
						"tags.datadoghq.com/env":              "production",
						"tags.datadoghq.com/service":          "pod-service",
						"tags.datadoghq.com/version":          "1.2.0",
						"tags.datadoghq.com/dd-agent.env":     "production",
						"tags.datadoghq.com/dd-agent.service": "dd-agent",
						"tags.datadoghq.com/dd-agent.version": "1.1.0",
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
			expectedInfo: []*TagInfo{{
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
					"env:production",
					"service:dd-agent",
					"service:pod-service",
					"version:1.1.0",
					"version:1.2.0",
					"pod_phase:running",
				},
				OrchestratorCardTags: []string{},
				HighCardTags: []string{
					"container_id:d0242fc32d53137526dc365e7c86ef43b5f50b6f72dfd53dcb948eff4560376f",
					"GitCommit:ea38b55f07e40b68177111a2bff1e918132fd5fb",
				},
			}},
		},
		{
			desc: "standard container env vars",
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
				Spec:   dockerContainerSpecWithEnv,
			},
			labelsAsTags: map[string]string{
				"component": "component",
				"tier":      "tier",
			},
			annotationsAsTags: map[string]string{
				"ownerteam": "team",
				"gitcommit": "+GitCommit",
			},
			expectedInfo: []*TagInfo{{
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
					"env:production",
					"service:dd-agent",
					"version:1.1.0",
					"pod_phase:running",
				},
				OrchestratorCardTags: []string{},
				HighCardTags: []string{
					"container_id:d0242fc32d53137526dc365e7c86ef43b5f50b6f72dfd53dcb948eff4560376f",
					"GitCommit:ea38b55f07e40b68177111a2bff1e918132fd5fb",
				},
			}},
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
			expectedInfo: []*TagInfo{{
				Source: "kubelet",
				Entity: dockerEntityID,
				LowCardTags: []string{
					"kube_container_name:dd-agent",
					"oshift_deployment_config:gitlab-ce",
					"pod_phase:running",
				},
				OrchestratorCardTags: []string{
					"oshift_deployment:gitlab-ce-1",
				},
				HighCardTags: []string{
					"container_id:d0242fc32d53137526dc365e7c86ef43b5f50b6f72dfd53dcb948eff4560376f",
				},
			}},
		},
		{
			desc: "CRI pod",
			pod: &kubelet.Pod{
				Metadata: kubelet.PodMetadata{
					Name: "redis-master-bpnn6",
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
			expectedInfo: []*TagInfo{{
				Source: "kubelet",
				Entity: criEntityID,
				LowCardTags: []string{
					"kube_container_name:redis-master",
					"kube_deployment:redis-master",
					"image_name:gcr.io/google_containers/redis",
					"image_tag:e2e",
					"short_image:redis",
					"pod_phase:running",
				},
				OrchestratorCardTags: []string{
					"kube_replica_set:redis-master-546dc4865f",
					"pod_name:redis-master-bpnn6",
				},
				HighCardTags: []string{
					"display_container_name:redis-master_redis-master-bpnn6",
					"container_id:acbe44ff07525934cab9bf7c38c6627d64fd0952d8e6b87535d57092bfa6e9d1",
				},
			}},
		},
		{
			desc: "pod labels as tags with wildcards",
			pod: &kubelet.Pod{
				Metadata: kubelet.PodMetadata{
					Labels: map[string]string{
						"component":                    "kube-proxy",
						"tier":                         "node",
						"k8s-app":                      "kubernetes-dashboard",
						"pod-template-hash":            "490794276",
						"app.kubernetes.io/managed-by": "spinnaker",
					},
				},
				Status: dockerContainerStatus,
				Spec:   dockerContainerSpec,
			},
			labelsAsTags: map[string]string{
				"*":         "foo_%%label%%",
				"component": "component",
			},
			annotationsAsTags: map[string]string{},
			expectedInfo: []*TagInfo{{
				Source: "kubelet",
				Entity: dockerEntityID,
				LowCardTags: []string{
					"foo_component:kube-proxy",
					"component:kube-proxy",
					"foo_tier:node",
					"foo_k8s-app:kubernetes-dashboard",
					"foo_pod-template-hash:490794276",
					"foo_app.kubernetes.io/managed-by:spinnaker",
					"image_name:datadog/docker-dd-agent",
					"image_tag:latest5",
					"kube_container_name:dd-agent",
					"short_image:docker-dd-agent",
					"pod_phase:running",
				},
				OrchestratorCardTags: []string{},
				HighCardTags:         []string{"container_id:d0242fc32d53137526dc365e7c86ef43b5f50b6f72dfd53dcb948eff4560376f"},
			}},
		}, {
			desc: "cronjob",
			pod: &kubelet.Pod{
				Metadata: kubelet.PodMetadata{
					Name:      "hello-1562187720-xzbzh",
					Namespace: "default",
					Owners: []kubelet.PodOwner{
						{
							Kind: "Job",
							Name: "hello-1562187720",
							ID:   "d0dcc17b-9dd5-11e9-b6f0-42010a840064",
						},
					},
				},
				Status: dockerContainerStatus,
				Spec:   dockerContainerSpec,
			},
			expectedInfo: []*TagInfo{{
				Source: "kubelet",
				Entity: dockerEntityID,
				LowCardTags: []string{
					"kube_namespace:default",
					"image_name:datadog/docker-dd-agent",
					"image_tag:latest5",
					"kube_container_name:dd-agent",
					"short_image:docker-dd-agent",
					"pod_phase:running",
					"kube_cronjob:hello",
				},
				OrchestratorCardTags: []string{"kube_job:hello-1562187720", "pod_name:hello-1562187720-xzbzh"},
				HighCardTags: []string{
					"container_id:d0242fc32d53137526dc365e7c86ef43b5f50b6f72dfd53dcb948eff4560376f",
					"display_container_name:dd-agent_hello-1562187720-xzbzh",
				},
			}},
		},
		{
			desc: "statefulset",
			pod: &kubelet.Pod{
				Metadata: kubelet.PodMetadata{
					Name:      "cassandra-0",
					Namespace: "default",
					Owners: []kubelet.PodOwner{
						{
							Kind: "StatefulSet",
							Name: "cassandra",
							ID:   "0fa7e650-da09-11e9-b8b8-42010af002dd",
						},
					},
				},
				Status: dockerContainerStatusCassandra,
				Spec:   dockerContainerSpecCassandra,
			},
			expectedInfo: []*TagInfo{{
				Source: "kubelet",
				Entity: dockerEntityIDCassandra,
				LowCardTags: []string{
					"kube_namespace:default",
					"image_name:gcr.io/google-samples/cassandra",
					"image_tag:v13",
					"kube_container_name:cassandra",
					"short_image:cassandra",
					"pod_phase:running",
					"kube_stateful_set:cassandra",
					"persistentvolumeclaim:cassandra-data-cassandra-0",
				},
				OrchestratorCardTags: []string{"pod_name:cassandra-0"},
				HighCardTags: []string{
					"container_id:6eaa4782de428f5ea639e33a837ed47aa9fa9e6969f8cb23e39ff788a751ce7d",
					"display_container_name:cassandra_cassandra-0",
				},
			}},
		},
		{
			desc: "statefulset 2 pvcs",
			pod: &kubelet.Pod{
				Metadata: kubelet.PodMetadata{
					Name:      "cassandra-0",
					Namespace: "default",
					Owners: []kubelet.PodOwner{
						{
							Kind: "StatefulSet",
							Name: "cassandra",
							ID:   "0fa7e650-da09-11e9-b8b8-42010af002dd",
						},
					},
				},
				Status: dockerContainerStatusCassandra,
				Spec:   dockerContainerSpecCassandraMultiplePvcs,
			},
			expectedInfo: []*TagInfo{{
				Source: "kubelet",
				Entity: dockerEntityIDCassandra,
				LowCardTags: []string{
					"kube_namespace:default",
					"image_name:gcr.io/google-samples/cassandra",
					"image_tag:v13",
					"kube_container_name:cassandra",
					"short_image:cassandra",
					"pod_phase:running",
					"kube_stateful_set:cassandra",
					"persistentvolumeclaim:cassandra-data-cassandra-0",
					"persistentvolumeclaim:another-pvc-data-0",
				},
				OrchestratorCardTags: []string{"pod_name:cassandra-0"},
				HighCardTags: []string{
					"container_id:6eaa4782de428f5ea639e33a837ed47aa9fa9e6969f8cb23e39ff788a751ce7d",
					"display_container_name:cassandra_cassandra-0",
				},
			}},
		},
		{
			desc: "multi-value tags",
			pod: &kubelet.Pod{
				Metadata: kubelet.PodMetadata{
					Annotations: map[string]string{
						"ad.datadoghq.com/tags":          `{"pod_template_version": "1.0.0", "team": ["A", "B"]}`,
						"ad.datadoghq.com/dd-agent.tags": `{"agent_version": "6.9.0", "python_version": ["2", "3"]}`,
					},
				},
				Status: dockerContainerStatus,
				Spec:   dockerContainerSpec,
			},
			expectedInfo: []*TagInfo{{
				Source: "kubelet",
				Entity: dockerEntityID,
				LowCardTags: []string{
					"kube_container_name:dd-agent",
					"image_tag:latest5",
					"image_name:datadog/docker-dd-agent",
					"short_image:docker-dd-agent",
					"pod_template_version:1.0.0",
					"team:A",
					"team:B",
					"agent_version:6.9.0",
					"python_version:2",
					"python_version:3",
					"pod_phase:running",
				},
				OrchestratorCardTags: []string{},
				HighCardTags: []string{
					"container_id:d0242fc32d53137526dc365e7c86ef43b5f50b6f72dfd53dcb948eff4560376f",
				},
			}},
		},
	} {
		t.Run(fmt.Sprintf("case %d: %s", nb, tc.desc), func(t *testing.T) {
			if tc.skip {
				t.SkipNow()
			}
			collector := &KubeletCollector{}
			collector.init(nil, nil, tc.labelsAsTags, tc.annotationsAsTags)
			infos, err := collector.parsePods([]*kubelet.Pod{tc.pod})
			assert.Nil(t, err)

			if tc.expectedInfo == nil {
				assert.Len(t, infos, 0)
			} else {
				assertTagInfoListEqual(t, tc.expectedInfo, infos)
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
			assert.Equal(t, out, parseDeploymentForReplicaset(in))
		})
	}
}

func TestParseCronJobForJob(t *testing.T) {
	for in, out := range map[string]string{
		"hello-1562319360": "hello",
		"hello-600":        "hello",
		"hello-world":      "",
		"hello":            "",
		"-hello1562319360": "",
		"hello1562319360":  "",
		"hello60":          "",
		"hello-60":         "",
		"hello-1562319a60": "",
	} {
		t.Run(fmt.Sprintf("case: %s", in), func(t *testing.T) {
			assert.Equal(t, out, parseCronJobForJob(in))
		})
	}
}

func Test_parseJSONValue(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    map[string][]string
		wantErr bool
	}{
		{
			name:    "empty json",
			value:   ``,
			want:    nil,
			wantErr: true,
		},
		{
			name:    "invalid json",
			value:   `{key}`,
			want:    nil,
			wantErr: true,
		},
		{
			name:  "invalid value",
			value: `{"key1": "val1", "key2": 0}`,
			want: map[string][]string{
				"key1": {"val1"},
			},
			wantErr: false,
		},
		{
			name:  "strings and arrays",
			value: `{"key1": "val1", "key2": ["val2"]}`,
			want: map[string][]string{
				"key1": {"val1"},
				"key2": {"val2"},
			},
			wantErr: false,
		},
		{
			name:  "arrays only",
			value: `{"key1": ["val1", "val11"], "key2": ["val2", "val22"]}`,
			want: map[string][]string{
				"key1": {"val1", "val11"},
				"key2": {"val2", "val22"},
			},
			wantErr: false,
		},
		{
			name:  "strings only",
			value: `{"key1": "val1", "key2": "val2"}`,
			want: map[string][]string{
				"key1": {"val1"},
				"key2": {"val2"},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseJSONValue(tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseJSONValue() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.Len(t, got, len(tt.want))
			for k, v := range tt.want {
				assert.ElementsMatch(t, v, got[k])
			}
		})
	}
}
