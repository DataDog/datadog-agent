// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collectors

import (
	"fmt"
	"sort"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/tagger/utils"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	workloadmetatesting "github.com/DataDog/datadog-agent/pkg/workloadmeta/testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
)

func TestHandleKubePod(t *testing.T) {
	const (
		fullyFleshedContainerID = "foobarquux"
		noEnvContainerID        = "foobarbaz"
		containerName           = "agent"
		runtimeContainerName    = "k8s_datadog-agent_agent"
		podName                 = "datadog-agent-foobar"
		podNamespace            = "default"
		env                     = "production"
		svc                     = "datadog-agent"
		version                 = "7.32.0"
	)

	standardTags := []string{
		fmt.Sprintf("env:%s", env),
		fmt.Sprintf("service:%s", svc),
		fmt.Sprintf("version:%s", version),
	}

	podEntityID := workloadmeta.EntityID{
		Kind: workloadmeta.KindKubernetesPod,
		ID:   "foobar",
	}

	podTaggerEntityID := fmt.Sprintf("kubernetes_pod_uid://%s", podEntityID.ID)
	fullyFleshedContainerTaggerEntityID := fmt.Sprintf("container_id://%s", fullyFleshedContainerID)
	noEnvContainerTaggerEntityID := fmt.Sprintf("container_id://%s", noEnvContainerID)

	image := workloadmeta.ContainerImage{
		ID:        "datadog/agent@sha256:a63d3f66fb2f69d955d4f2ca0b229385537a77872ffc04290acae65aed5317d2",
		RawName:   "datadog/agent@sha256:a63d3f66fb2f69d955d4f2ca0b229385537a77872ffc04290acae65aed5317d2",
		Name:      "datadog/agent",
		ShortName: "agent",
		Tag:       "latest",
	}

	store := workloadmetatesting.NewStore()
	store.Set(&workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   fullyFleshedContainerID,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: runtimeContainerName,
		},
		Image: image,
		EnvVars: map[string]string{
			"DD_ENV":     env,
			"DD_SERVICE": svc,
			"DD_VERSION": version,
		},
	})
	store.Set(&workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   noEnvContainerID,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: runtimeContainerName,
		},
	})

	tests := []struct {
		name              string
		staticTags        map[string]string
		labelsAsTags      map[string]string
		annotationsAsTags map[string]string
		nsLabelsAsTags    map[string]string
		pod               workloadmeta.KubernetesPod
		expected          []*TagInfo
	}{
		{
			name: "fully formed pod (no containers)",
			annotationsAsTags: map[string]string{
				"gitcommit": "+gitcommit",
				"component": "component",
			},
			labelsAsTags: map[string]string{
				"ownerteam": "team",
				"tier":      "tier",
			},
			nsLabelsAsTags: map[string]string{
				"ns_env":       "ns_env",
				"ns-ownerteam": "ns-team",
			},
			pod: workloadmeta.KubernetesPod{
				EntityID: podEntityID,
				EntityMeta: workloadmeta.EntityMeta{
					Name:      podName,
					Namespace: podNamespace,
					Annotations: map[string]string{
						// Annotations as tags
						"GitCommit": "foobar",
						"ignoreme":  "ignore",
						"component": "agent",

						// Custom tags from map
						"ad.datadoghq.com/tags": `{"pod_template_version":"1.0.0"}`,
					},
					Labels: map[string]string{
						// Labels as tags
						"OwnerTeam":         "container-integrations",
						"tier":              "node",
						"pod-template-hash": "490794276",

						// Standard tags
						"tags.datadoghq.com/env":     env,
						"tags.datadoghq.com/service": svc,
						"tags.datadoghq.com/version": version,

						// K8s recommended tags
						"app.kubernetes.io/name":       svc,
						"app.kubernetes.io/instance":   podName,
						"app.kubernetes.io/version":    version,
						"app.kubernetes.io/component":  "agent",
						"app.kubernetes.io/part-of":    "datadog",
						"app.kubernetes.io/managed-by": "helm",
					},
				},

				// NS labels as tags
				NamespaceLabels: map[string]string{
					"ns_env":       "dev",
					"ns-ownerteam": "containers",
					"foo":          "bar",
				},

				// kube_service tags
				KubeServices: []string{"service1", "service2"},

				// Owner tags
				Owners: []workloadmeta.KubernetesPodOwner{
					{
						Kind: kubernetes.DeploymentKind,
						Name: svc,
					},
				},

				// PVC tags
				PersistentVolumeClaimNames: []string{"pvc-0"},

				// QOS tags
				QOSClass: "guaranteed",

				// Phase tags
				Phase: "Running",
			},
			expected: []*TagInfo{
				{
					Source: podSource,
					Entity: podTaggerEntityID,
					HighCardTags: []string{
						"gitcommit:foobar",
					},
					OrchestratorCardTags: []string{
						fmt.Sprintf("pod_name:%s", podName),
						"kube_ownerref_name:datadog-agent",
					},
					LowCardTags: append([]string{
						fmt.Sprintf("kube_app_instance:%s", podName),
						fmt.Sprintf("kube_app_name:%s", svc),
						fmt.Sprintf("kube_app_version:%s", version),
						fmt.Sprintf("kube_deployment:%s", svc),
						fmt.Sprintf("kube_namespace:%s", podNamespace),
						"component:agent",
						"kube_app_component:agent",
						"kube_app_managed_by:helm",
						"kube_app_part_of:datadog",
						"kube_ownerref_kind:deployment",
						"kube_service:service1",
						"kube_service:service2",
						"kube_qos:guaranteed",
						"ns-team:containers",
						"ns_env:dev",
						"pod_phase:running",
						"pod_template_version:1.0.0",
						"team:container-integrations",
						"tier:node",
					}, standardTags...),
					StandardTags: standardTags,
				},
			},
		},
		{
			name: "pod with fully formed container, standard tags from env",
			pod: workloadmeta.KubernetesPod{
				EntityID: podEntityID,
				EntityMeta: workloadmeta.EntityMeta{
					Name:      podName,
					Namespace: podNamespace,
				},
				Containers: []workloadmeta.OrchestratorContainer{
					{
						ID:    fullyFleshedContainerID,
						Name:  containerName,
						Image: image,
					},
				},
			},
			expected: []*TagInfo{
				{
					Source:       podSource,
					Entity:       podTaggerEntityID,
					HighCardTags: []string{},
					OrchestratorCardTags: []string{
						fmt.Sprintf("pod_name:%s", podName),
					},
					LowCardTags: []string{
						fmt.Sprintf("kube_namespace:%s", podNamespace),
					},
					StandardTags: []string{},
				},
				{
					Source: podSource,
					Entity: fullyFleshedContainerTaggerEntityID,
					HighCardTags: []string{
						fmt.Sprintf("container_id:%s", fullyFleshedContainerID),
						fmt.Sprintf("display_container_name:%s_%s", runtimeContainerName, podName),
					},
					OrchestratorCardTags: []string{
						fmt.Sprintf("pod_name:%s", podName),
					},
					LowCardTags: append([]string{
						fmt.Sprintf("kube_namespace:%s", podNamespace),
						fmt.Sprintf("kube_container_name:%s", containerName),
						"image_id:datadog/agent@sha256:a63d3f66fb2f69d955d4f2ca0b229385537a77872ffc04290acae65aed5317d2",
						"image_name:datadog/agent",
						"image_tag:latest",
						"short_image:agent",
					}, standardTags...),
					StandardTags: standardTags,
				},
			},
		},
		{
			name: "pod with container, standard tags from labels",
			pod: workloadmeta.KubernetesPod{
				EntityID: podEntityID,
				EntityMeta: workloadmeta.EntityMeta{
					Name:      podName,
					Namespace: podNamespace,
					Labels: map[string]string{
						"tags.datadoghq.com/agent.env":     env,
						"tags.datadoghq.com/agent.service": svc,
						"tags.datadoghq.com/agent.version": version,
					},
				},
				Containers: []workloadmeta.OrchestratorContainer{
					{
						ID:   noEnvContainerID,
						Name: containerName,
					},
				},
			},
			expected: []*TagInfo{
				{
					Source:       podSource,
					Entity:       podTaggerEntityID,
					HighCardTags: []string{},
					OrchestratorCardTags: []string{
						fmt.Sprintf("pod_name:%s", podName),
					},
					LowCardTags: []string{
						fmt.Sprintf("kube_namespace:%s", podNamespace),
					},
					StandardTags: []string{},
				},
				{
					Source: podSource,
					Entity: noEnvContainerTaggerEntityID,
					HighCardTags: []string{
						fmt.Sprintf("container_id:%s", noEnvContainerID),
						fmt.Sprintf("display_container_name:%s_%s", runtimeContainerName, podName),
					},
					OrchestratorCardTags: []string{
						fmt.Sprintf("pod_name:%s", podName),
					},
					LowCardTags: append([]string{
						fmt.Sprintf("kube_namespace:%s", podNamespace),
						fmt.Sprintf("kube_container_name:%s", containerName),
					}, standardTags...),
					StandardTags: standardTags,
				},
			},
		},
		{
			name: "pod from openshift deployment",
			pod: workloadmeta.KubernetesPod{
				EntityID: podEntityID,
				EntityMeta: workloadmeta.EntityMeta{
					Name:      podName,
					Namespace: podNamespace,
					Annotations: map[string]string{
						"openshift.io/deployment-config.latest-version": "1",
						"openshift.io/deployment-config.name":           "gitlab-ce",
						"openshift.io/deployment.name":                  "gitlab-ce-1",
					},
				},
			},
			expected: []*TagInfo{
				{
					Source:       podSource,
					Entity:       podTaggerEntityID,
					HighCardTags: []string{},
					OrchestratorCardTags: []string{
						fmt.Sprintf("pod_name:%s", podName),
						"oshift_deployment:gitlab-ce-1",
					},
					LowCardTags: []string{
						fmt.Sprintf("kube_namespace:%s", podNamespace),
						"oshift_deployment_config:gitlab-ce",
					},
					StandardTags: []string{},
				},
			},
		},
		{
			name: "pod with admission + remote config annotations",
			pod: workloadmeta.KubernetesPod{
				EntityID: podEntityID,
				EntityMeta: workloadmeta.EntityMeta{
					Name:      podName,
					Namespace: podNamespace,
					Annotations: map[string]string{
						"admission.datadoghq.com/rc.id":  "id",
						"admission.datadoghq.com/rc.rev": "123",
					},
				},
			},
			expected: []*TagInfo{
				{
					Source:       podSource,
					Entity:       podTaggerEntityID,
					HighCardTags: []string{},
					OrchestratorCardTags: []string{
						fmt.Sprintf("pod_name:%s", podName),
					},
					LowCardTags: []string{
						fmt.Sprintf("kube_namespace:%s", podNamespace),
						"dd_remote_config_id:id",
						"dd_remote_config_rev:123",
					},
					StandardTags: []string{},
				},
			},
		},
		{
			name: "static tags",
			staticTags: map[string]string{
				"eks_fargate_node": "foobar",
			},
			pod: workloadmeta.KubernetesPod{
				EntityID: podEntityID,
				EntityMeta: workloadmeta.EntityMeta{
					Name:      podName,
					Namespace: podNamespace,
				},
			},
			expected: []*TagInfo{
				{
					Source:       podSource,
					Entity:       podTaggerEntityID,
					HighCardTags: []string{},
					OrchestratorCardTags: []string{
						fmt.Sprintf("pod_name:%s", podName),
					},
					LowCardTags: []string{
						fmt.Sprintf("kube_namespace:%s", podNamespace),
						"eks_fargate_node:foobar",
					},
					StandardTags: []string{},
				},
			},
		},
		{
			name: "disable kube_service",
			pod: workloadmeta.KubernetesPod{
				EntityID: podEntityID,
				EntityMeta: workloadmeta.EntityMeta{
					Name:      podName,
					Namespace: podNamespace,
					Annotations: map[string]string{
						"tags.datadoghq.com/disable": "kube_service",
					},
				},
				// kube_service tags
				KubeServices: []string{"service1", "service2"},
			},
			expected: []*TagInfo{
				{
					Source:       podSource,
					Entity:       podTaggerEntityID,
					HighCardTags: []string{},
					OrchestratorCardTags: []string{
						fmt.Sprintf("pod_name:%s", podName),
					},
					LowCardTags: []string{
						fmt.Sprintf("kube_namespace:%s", podNamespace),
					},
					StandardTags: []string{},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			collector := &WorkloadMetaCollector{
				store:      store,
				children:   make(map[string]map[string]struct{}),
				staticTags: tt.staticTags,
			}

			collector.initPodMetaAsTags(tt.labelsAsTags, tt.annotationsAsTags, tt.nsLabelsAsTags)

			actual := collector.handleKubePod(workloadmeta.Event{
				Type:   workloadmeta.EventTypeSet,
				Entity: &tt.pod,
			})

			assertTagInfoListEqual(t, tt.expected, actual)
		})
	}
}

func TestHandleECSTask(t *testing.T) {
	const (
		containerID   = "foobarquux"
		containerName = "agent"
	)

	entityID := workloadmeta.EntityID{
		Kind: workloadmeta.KindECSTask,
		ID:   "foobar",
	}

	taggerEntityID := fmt.Sprintf("container_id://%s", containerID)

	store := workloadmetatesting.NewStore()
	store.Set(&workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   containerID,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: containerName,
		},
	})

	tests := []struct {
		name     string
		task     workloadmeta.ECSTask
		expected []*TagInfo
	}{
		{
			name: "basic ECS EC2 task",
			task: workloadmeta.ECSTask{
				EntityID: entityID,
				EntityMeta: workloadmeta.EntityMeta{
					Name: "foobar",
				},
				Tags: map[string]string{
					"aws:ecs:clusterName": "ecs-cluster",
					"aws:ecs:serviceName": "datadog-agent",
					"owner_team":          "container-integrations",
				},
				ContainerInstanceTags: map[string]string{
					"instance_type": "g4dn.xlarge",
				},
				ClusterName: "ecs-cluster",
				Family:      "datadog-agent",
				Version:     "1",
				LaunchType:  workloadmeta.ECSLaunchTypeEC2,
				Containers: []workloadmeta.OrchestratorContainer{
					{
						ID:   containerID,
						Name: containerName,
					},
				},
			},
			expected: []*TagInfo{
				{
					Source:       taskSource,
					Entity:       taggerEntityID,
					HighCardTags: []string{},
					OrchestratorCardTags: []string{
						"task_arn:foobar",
					},
					LowCardTags: []string{
						"cluster_name:ecs-cluster",
						"ecs_cluster_name:ecs-cluster",
						"ecs_container_name:agent",
						"instance_type:g4dn.xlarge",
						"owner_team:container-integrations",
						"task_family:datadog-agent",
						"task_name:datadog-agent",
						"task_version:1",
					},
					StandardTags: []string{},
				},
			},
		},
		{
			name: "basic ECS Fargate task",
			task: workloadmeta.ECSTask{
				EntityID: entityID,
				EntityMeta: workloadmeta.EntityMeta{
					Name: "foobar",
				},
				ClusterName: "ecs-cluster",
				Family:      "datadog-agent",
				Version:     "1",
				LaunchType:  workloadmeta.ECSLaunchTypeFargate,
				Containers: []workloadmeta.OrchestratorContainer{
					{
						ID:   containerID,
						Name: containerName,
					},
				},
				AvailabilityZone: "us-east-1c",
			},
			expected: []*TagInfo{
				{
					Source:       taskSource,
					Entity:       taggerEntityID,
					HighCardTags: []string{},
					OrchestratorCardTags: []string{
						"task_arn:foobar",
					},
					LowCardTags: []string{
						"cluster_name:ecs-cluster",
						"ecs_cluster_name:ecs-cluster",
						"ecs_container_name:agent",
						"task_family:datadog-agent",
						"task_name:datadog-agent",
						"task_version:1",
						"availability_zone:us-east-1c",
						"availability-zone:us-east-1c",
					},
					StandardTags: []string{},
				},
				{
					Source:       taskSource,
					Entity:       GlobalEntityID,
					HighCardTags: []string{},
					OrchestratorCardTags: []string{
						"task_arn:foobar",
					},
					LowCardTags: []string{
						"cluster_name:ecs-cluster",
						"ecs_cluster_name:ecs-cluster",
						"task_family:datadog-agent",
						"task_name:datadog-agent",
						"task_version:1",
						"availability_zone:us-east-1c",
						"availability-zone:us-east-1c",
					},
					StandardTags: []string{},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			collector := &WorkloadMetaCollector{
				store:                  store,
				children:               make(map[string]map[string]struct{}),
				collectEC2ResourceTags: true,
			}

			actual := collector.handleECSTask(workloadmeta.Event{
				Type:   workloadmeta.EventTypeSet,
				Entity: &tt.task,
			})

			assertTagInfoListEqual(t, tt.expected, actual)
		})
	}
}

func TestHandleContainer(t *testing.T) {
	const (
		containerName = "foobar"
		podNamespace  = "default"
		env           = "production"
		svc           = "datadog-agent"
		version       = "7.32.0"
	)

	standardTags := []string{
		fmt.Sprintf("env:%s", env),
		fmt.Sprintf("service:%s", svc),
		fmt.Sprintf("version:%s", version),
	}

	entityID := workloadmeta.EntityID{
		Kind: workloadmeta.KindContainer,
		ID:   "foobar",
	}

	taggerEntityID := fmt.Sprintf("container_id://%s", entityID.ID)

	tests := []struct {
		name         string
		staticTags   map[string]string
		labelsAsTags map[string]string
		envAsTags    map[string]string
		container    workloadmeta.Container
		expected     []*TagInfo
	}{
		{
			name: "fully formed container",
			container: workloadmeta.Container{
				EntityID: entityID,
				EntityMeta: workloadmeta.EntityMeta{
					Name: containerName,
					Labels: map[string]string{
						"com.datadoghq.tags.env":     env,
						"com.datadoghq.tags.service": svc,
						"com.datadoghq.tags.version": version,
					},
				},
				Runtime: workloadmeta.ContainerRuntimeDocker,
				Image: workloadmeta.ContainerImage{
					ID:        "datadog/agent@sha256:a63d3f66fb2f69d955d4f2ca0b229385537a77872ffc04290acae65aed5317d2",
					RawName:   "datadog/agent@sha256:a63d3f66fb2f69d955d4f2ca0b229385537a77872ffc04290acae65aed5317d2",
					Name:      "datadog/agent",
					ShortName: "agent",
					Tag:       "latest",
				},
			},
			expected: []*TagInfo{
				{
					Source: containerSource,
					Entity: taggerEntityID,
					HighCardTags: []string{
						fmt.Sprintf("container_name:%s", containerName),
						fmt.Sprintf("container_id:%s", entityID.ID),
					},
					OrchestratorCardTags: []string{},
					LowCardTags: append([]string{
						"docker_image:datadog/agent:latest",
						"image_name:datadog/agent",
						"image_tag:latest",
						"short_image:agent",
						"image_id:datadog/agent@sha256:a63d3f66fb2f69d955d4f2ca0b229385537a77872ffc04290acae65aed5317d2",
					}, standardTags...),
					StandardTags: standardTags,
				},
			},
		},
		{
			name: "tags from environment",
			container: workloadmeta.Container{
				EntityID: entityID,
				EntityMeta: workloadmeta.EntityMeta{
					Name: containerName,
				},
				EnvVars: map[string]string{
					// env as tags
					"TEAM": "container-integrations",
					"TIER": "node",

					// standard tags
					"DD_ENV":     env,
					"DD_SERVICE": svc,
					"DD_VERSION": version,
				},
			},
			envAsTags: map[string]string{
				"team": "owner_team",
			},
			expected: []*TagInfo{
				{
					Source: containerSource,
					Entity: taggerEntityID,
					HighCardTags: []string{
						fmt.Sprintf("container_name:%s", containerName),
						fmt.Sprintf("container_id:%s", entityID.ID),
					},
					OrchestratorCardTags: []string{},
					LowCardTags: append([]string{
						"owner_team:container-integrations",
					}, standardTags...),
					StandardTags: standardTags,
				},
			},
		},
		{
			name: "tags from labels",
			container: workloadmeta.Container{
				EntityID: entityID,
				EntityMeta: workloadmeta.EntityMeta{
					Name: containerName,
					Labels: map[string]string{
						// labels as tags
						"team": "container-integrations",
						"tier": "node",

						// custom tags from label
						"com.datadoghq.ad.tags": `["app_name:datadog-agent"]`,
					},
				},
			},
			labelsAsTags: map[string]string{
				"team": "owner_team",
			},
			expected: []*TagInfo{
				{
					Source: containerSource,
					Entity: taggerEntityID,
					HighCardTags: []string{
						fmt.Sprintf("container_name:%s", containerName),
						fmt.Sprintf("container_id:%s", entityID.ID),
						"app_name:datadog-agent",
					},
					OrchestratorCardTags: []string{},
					LowCardTags: []string{
						"owner_team:container-integrations",
					},
					StandardTags: []string{},
				},
			},
		},
		{
			name: "tags from labels and envs with prefix (using *)",
			container: workloadmeta.Container{
				EntityID: entityID,
				EntityMeta: workloadmeta.EntityMeta{
					Name: containerName,
					Labels: map[string]string{
						"team": "container-integrations",
					},
				},
				EnvVars: map[string]string{
					"some_env": "some_env_val",
				},
			},
			labelsAsTags: map[string]string{
				"*": "custom_label_prefix_%%label%%",
			},
			envAsTags: map[string]string{
				"*": "custom_env_prefix_%%env%%",
			},
			expected: []*TagInfo{
				{
					Source: containerSource,
					Entity: taggerEntityID,
					HighCardTags: []string{
						fmt.Sprintf("container_name:%s", containerName),
						fmt.Sprintf("container_id:%s", entityID.ID),
					},
					OrchestratorCardTags: []string{},
					LowCardTags: []string{
						// Notice that the names include the custom prefixes
						// added in labelsAsTags and envAsTags.
						"custom_label_prefix_team:container-integrations",
						"custom_env_prefix_some_env:some_env_val",
					},
					StandardTags: []string{},
				},
			},
		},
		{
			name: "docker container with image that has no tag",
			container: workloadmeta.Container{
				EntityID: entityID,
				EntityMeta: workloadmeta.EntityMeta{
					Name: containerName,
				},
				Runtime: workloadmeta.ContainerRuntimeDocker,
				Image: workloadmeta.ContainerImage{
					RawName:   "redis",
					Name:      "redis",
					ShortName: "redis",
					Tag:       "",
				},
			},
			expected: []*TagInfo{
				{
					Source: containerSource,
					Entity: taggerEntityID,
					HighCardTags: []string{
						fmt.Sprintf("container_name:%s", containerName),
						fmt.Sprintf("container_id:%s", entityID.ID),
					},
					OrchestratorCardTags: []string{},
					LowCardTags: []string{
						"docker_image:redis", // Notice that there's no tag
						"image_name:redis",
						"short_image:redis",
					},
					StandardTags: []string{},
				},
			},
		},
		{
			name: "nomad container",
			container: workloadmeta.Container{
				EntityID: entityID,
				EntityMeta: workloadmeta.EntityMeta{
					Name: containerName,
				},
				EnvVars: map[string]string{
					"NOMAD_TASK_NAME":  "test-task",
					"NOMAD_JOB_NAME":   "test-job",
					"NOMAD_GROUP_NAME": "test-group",
					"NOMAD_NAMESPACE":  "test-namespace",
					"NOMAD_DC":         "test-dc",
				},
			},
			expected: []*TagInfo{
				{
					Source: containerSource,
					Entity: taggerEntityID,
					HighCardTags: []string{
						fmt.Sprintf("container_name:%s", containerName),
						fmt.Sprintf("container_id:%s", entityID.ID),
					},
					OrchestratorCardTags: []string{},
					LowCardTags: []string{
						"nomad_task:test-task",
						"nomad_job:test-job",
						"nomad_group:test-group",
						"nomad_namespace:test-namespace",
						"nomad_dc:test-dc",
					},
					StandardTags: []string{},
				},
			},
		},
		{
			name: "mesos dc/os container",
			container: workloadmeta.Container{
				EntityID: entityID,
				EntityMeta: workloadmeta.EntityMeta{
					Name: containerName,
				},
				EnvVars: map[string]string{
					"MARATHON_APP_ID":   "/system/dd-agent",
					"CHRONOS_JOB_NAME":  "app1_process-orders",
					"CHRONOS_JOB_OWNER": "qa",
					"MESOS_TASK_ID":     "system_dd-agent.dcc75b42-4b87-11e7-9a62-70b3d5800001",
				},
			},
			expected: []*TagInfo{
				{
					Source: containerSource,
					Entity: taggerEntityID,
					HighCardTags: []string{
						fmt.Sprintf("container_name:%s", containerName),
						fmt.Sprintf("container_id:%s", entityID.ID),
					},
					OrchestratorCardTags: []string{
						"mesos_task:system_dd-agent.dcc75b42-4b87-11e7-9a62-70b3d5800001",
					},
					LowCardTags: []string{
						"chronos_job:app1_process-orders",
						"chronos_job_owner:qa",
						"marathon_app:/system/dd-agent",
					},
					StandardTags: []string{},
				},
			},
		},
		{
			name: "rancher container",
			container: workloadmeta.Container{
				EntityID: entityID,
				EntityMeta: workloadmeta.EntityMeta{
					Name: containerName,
					Labels: map[string]string{
						"io.rancher.cni.network":             "ipsec",
						"io.rancher.cni.wait":                "true",
						"io.rancher.container.ip":            "10.42.234.7/16",
						"io.rancher.container.mac_address":   "02:f1:dd:48:4c:d9",
						"io.rancher.container.name":          "testAD-redis-1",
						"io.rancher.container.pull_image":    "always",
						"io.rancher.container.uuid":          "8e969193-2bc7-4a58-9a54-9eed44b01bb2",
						"io.rancher.environment.uuid":        "adminProject",
						"io.rancher.project.name":            "testAD",
						"io.rancher.project_service.name":    "testAD/redis",
						"io.rancher.service.deployment.unit": "06c082fc-4b66-4b6c-b098-30dbf29ed204",
						"io.rancher.service.launch.config":   "io.rancher.service.primary.launch.config",
						"io.rancher.stack.name":              "testAD",
						"io.rancher.stack_service.name":      "testAD/redis",
					},
				},
			},
			expected: []*TagInfo{
				{
					Source: containerSource,
					Entity: taggerEntityID,
					HighCardTags: []string{
						fmt.Sprintf("container_name:%s", containerName),
						fmt.Sprintf("container_id:%s", entityID.ID),
						"rancher_container:testAD-redis-1",
					},
					OrchestratorCardTags: []string{},
					LowCardTags: []string{
						"rancher_service:testAD/redis",
						"rancher_stack:testAD",
					},
					StandardTags: []string{},
				},
			},
		},
		{
			name: "docker swarm container",
			container: workloadmeta.Container{
				EntityID: entityID,
				EntityMeta: workloadmeta.EntityMeta{
					Name: containerName,
					Labels: map[string]string{
						"com.docker.swarm.node.id":      "zdtab51ei97djzrpa1y2tz8li",
						"com.docker.swarm.service.id":   "tef96xrdmlj82c7nt57jdntl8",
						"com.docker.swarm.service.name": "helloworld",
						"com.docker.swarm.task":         "",
						"com.docker.swarm.task.id":      "knk1rz1szius7pvyznn9zolld",
						"com.docker.swarm.task.name":    "helloworld.1.knk1rz1szius7pvyznn9zolld",
						"com.docker.stack.namespace":    "default",
					},
				},
			},
			expected: []*TagInfo{
				{
					Source: containerSource,
					Entity: taggerEntityID,
					HighCardTags: []string{
						fmt.Sprintf("container_name:%s", containerName),
						fmt.Sprintf("container_id:%s", entityID.ID),
					},
					OrchestratorCardTags: []string{},
					LowCardTags: []string{
						"swarm_namespace:default",
						"swarm_service:helloworld",
					},
					StandardTags: []string{},
				},
			},
		},
		{
			name: "opencontainers image revision and source",
			container: workloadmeta.Container{
				EntityID: entityID,
				EntityMeta: workloadmeta.EntityMeta{
					Name: containerName,
					Labels: map[string]string{
						"org.opencontainers.image.revision": "758691a28aa920070651d360814c559bc26af907",
						"org.opencontainers.image.source":   "https://github.com/my-company/repo",
					},
				},
			},
			expected: []*TagInfo{
				{
					Source: containerSource,
					Entity: taggerEntityID,
					HighCardTags: []string{
						fmt.Sprintf("container_name:%s", containerName),
						fmt.Sprintf("container_id:%s", entityID.ID),
					},
					OrchestratorCardTags: []string{},
					LowCardTags: []string{
						"git.commit.sha:758691a28aa920070651d360814c559bc26af907",
						"git.repository_url:https://github.com/my-company/repo",
					},
					StandardTags: []string{},
				},
			},
		},
		{
			name: "static tags",
			staticTags: map[string]string{
				"eks_fargate_node": "foobar",
			},
			container: workloadmeta.Container{
				EntityID: entityID,
				EntityMeta: workloadmeta.EntityMeta{
					Name: containerName,
				},
			},
			expected: []*TagInfo{
				{
					Source: containerSource,
					Entity: taggerEntityID,
					HighCardTags: []string{
						fmt.Sprintf("container_name:%s", containerName),
						fmt.Sprintf("container_id:%s", entityID.ID),
					},
					OrchestratorCardTags: []string{},
					LowCardTags: []string{
						"eks_fargate_node:foobar",
					},
					StandardTags: []string{},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			collector := &WorkloadMetaCollector{
				staticTags: tt.staticTags,
			}
			collector.initContainerMetaAsTags(tt.labelsAsTags, tt.envAsTags)

			actual := collector.handleContainer(workloadmeta.Event{
				Type:   workloadmeta.EventTypeSet,
				Entity: &tt.container,
			})

			assertTagInfoListEqual(t, tt.expected, actual)
		})
	}
}

func TestHandleContainerImage(t *testing.T) {
	entityID := workloadmeta.EntityID{
		Kind: workloadmeta.KindContainerImageMetadata,
		ID:   "sha256:651c55002cd5deb06bde7258f6ec6e0ff7f4f17a648ce6e2ec01917da9ae5104",
	}

	taggerEntityID := fmt.Sprintf("container_image_metadata://%s", entityID.ID)

	tests := []struct {
		name     string
		image    workloadmeta.ContainerImageMetadata
		expected []*TagInfo
	}{
		{
			name: "basic",
			image: workloadmeta.ContainerImageMetadata{
				EntityID: entityID,
				EntityMeta: workloadmeta.EntityMeta{
					Name: entityID.ID,
					Labels: map[string]string{
						"com.datadoghq.tags.env":     "production",
						"com.datadoghq.tags.service": "datadog-agent",
						"com.datadoghq.tags.version": "8.0.0",
					},
				},
				RepoTags: []string{
					"datadog/agent:7.41.1-rc.1",
					"gcr.io/datadoghq/agent:7-rc",
					"gcr.io/datadoghq/agent:7.41.1-rc.1",
					"public.ecr.aws/datadog/agent:7-rc",
					"public.ecr.aws/datadog/agent:7.41.1-rc.1",
				},
				RepoDigests: []string{
					"datadog/agent@sha256:052f1fdf4f9a7117d36a1838ab60782829947683007c34b69d4991576375c409",
					"gcr.io/datadoghq/agent@sha256:052f1fdf4f9a7117d36a1838ab60782829947683007c34b69d4991576375c409",
					"public.ecr.aws/datadog/agent@sha256:052f1fdf4f9a7117d36a1838ab60782829947683007c34b69d4991576375c409",
				},
				OS:           "DOS",
				OSVersion:    "6.22",
				Architecture: "80486DX",
			},
			expected: []*TagInfo{
				{
					Source:               containerImageSource,
					Entity:               taggerEntityID,
					HighCardTags:         []string{},
					OrchestratorCardTags: []string{},
					LowCardTags: []string{
						"architecture:80486DX",
						"env:production",
						"image_name:sha256:651c55002cd5deb06bde7258f6ec6e0ff7f4f17a648ce6e2ec01917da9ae5104",
						"image_tag:7-rc",
						"image_tag:7.41.1-rc.1",
						"os_name:DOS",
						"os_version:6.22",
						"service:datadog-agent",
						"short_image:agent",
						"version:8.0.0",
					},
					StandardTags: []string{
						"env:production",
						"service:datadog-agent",
						"version:8.0.0",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			collector := &WorkloadMetaCollector{}

			actual := collector.handleContainerImage(workloadmeta.Event{
				Type:   workloadmeta.EventTypeSet,
				Entity: &tt.image,
			})

			assertTagInfoListEqual(t, tt.expected, actual)
		})
	}
}

func TestHandleDelete(t *testing.T) {
	const (
		podName       = "datadog-agent-foobar"
		podNamespace  = "default"
		containerID   = "foobarquux"
		containerName = "agent"
	)

	podEntityID := workloadmeta.EntityID{
		Kind: workloadmeta.KindKubernetesPod,
		ID:   "foobar",
	}
	pod := &workloadmeta.KubernetesPod{
		EntityID: podEntityID,
		EntityMeta: workloadmeta.EntityMeta{
			Name:      podName,
			Namespace: podNamespace,
		},
		Containers: []workloadmeta.OrchestratorContainer{
			{
				ID:   containerID,
				Name: containerName,
			},
		},
	}

	podTaggerEntityID := fmt.Sprintf("kubernetes_pod_uid://%s", podEntityID.ID)
	containerTaggerEntityID := fmt.Sprintf("container_id://%s", containerID)

	store := workloadmetatesting.NewStore()
	store.Set(&workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   containerID,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: containerName,
		},
	})

	collector := &WorkloadMetaCollector{
		store:    store,
		children: make(map[string]map[string]struct{}),
	}

	collector.handleKubePod(workloadmeta.Event{
		Type:   workloadmeta.EventTypeSet,
		Entity: pod,
	})

	expected := []*TagInfo{
		{
			Source:       podSource,
			Entity:       podTaggerEntityID,
			DeleteEntity: true,
		},
		{
			Source:       podSource,
			Entity:       containerTaggerEntityID,
			DeleteEntity: true,
		},
	}

	actual := collector.handleDelete(workloadmeta.Event{
		Type:   workloadmeta.EventTypeUnset,
		Entity: pod,
	})

	assertTagInfoListEqual(t, expected, actual)
	assert.Empty(t, collector.children)
}

type fakeProcessor struct {
	ch chan []*TagInfo
}

func (p *fakeProcessor) ProcessTagInfo(tagInfos []*TagInfo) {
	p.ch <- tagInfos
}

func TestHandlePodWithDeletedContainer(t *testing.T) {
	// This test checks that we get events to delete a container that no longer
	// exists even if it belonged to a pod that still exists.

	containerToBeDeletedID := "delete"
	containerToBeDeletedTaggerEntityID := fmt.Sprintf("container_id://%s", containerToBeDeletedID)

	pod := &workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   "123",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:      "datadog-agent",
			Namespace: "default",
		},
		Containers: []workloadmeta.OrchestratorContainer{},
	}
	podTaggerEntityID := fmt.Sprintf("kubernetes_pod_uid://%s", pod.ID)

	collectorCh := make(chan []*TagInfo, 10)

	collector := &WorkloadMetaCollector{
		store: workloadmetatesting.NewStore(),
		children: map[string]map[string]struct{}{
			// Notice that here we set the container that belonged to the pod
			// but that no longer exists
			podTaggerEntityID: {
				containerToBeDeletedTaggerEntityID: struct{}{},
			},
		},
		tagProcessor: &fakeProcessor{collectorCh},
	}

	eventBundle := workloadmeta.EventBundle{
		Events: []workloadmeta.Event{
			{
				Type:   workloadmeta.EventTypeSet,
				Entity: pod,
			},
		},
		Ch: make(chan struct{}),
	}

	collector.processEvents(eventBundle)
	close(collectorCh)

	expected := &TagInfo{
		Source:       podSource,
		Entity:       containerToBeDeletedTaggerEntityID,
		DeleteEntity: true,
	}

	// We should receive an event to set the pod and another to delete the
	// container. Here we're only interested in the latter, because the former
	// is already checked in other tests.
	found := false
	for evBundle := range collectorCh {
		for _, event := range evBundle {
			if cmp.Equal(event, expected) {
				found = true
				break
			}
		}
	}

	assert.True(t, found, "TagInfo of deleted container not returned")
}

func TestParseJSONValue(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    []string
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
			want: []string{
				"key1:val1",
			},
			wantErr: false,
		},
		{
			name:  "strings and arrays",
			value: `{"key1": "val1", "key2": ["val2"]}`,
			want: []string{
				"key1:val1",
				"key2:val2",
			},
			wantErr: false,
		},
		{
			name:  "arrays only",
			value: `{"key1": ["val1", "val11"], "key2": ["val2", "val22"]}`,
			want: []string{
				"key1:val1",
				"key1:val11",
				"key2:val2",
				"key2:val22",
			},
			wantErr: false,
		},
		{
			name:  "strings only",
			value: `{"key1": "val1", "key2": "val2"}`,
			want: []string{
				"key1:val1",
				"key2:val2",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tags := utils.NewTagList()
			err := parseJSONValue(tt.value, tags)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseJSONValue() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			low, _, _, _ := tags.Compute()
			assert.ElementsMatch(t, tt.want, low)
		})
	}
}

func Test_mergeMaps(t *testing.T) {
	tests := []struct {
		name   string
		first  map[string]string
		second map[string]string
		want   map[string]string
	}{
		{
			name:   "no conflict",
			first:  map[string]string{"first-k1": "first-v1", "first-k2": "first-v2"},
			second: map[string]string{"second-k1": "second-v1", "second-k2": "second-v2"},
			want: map[string]string{
				"first-k1":  "first-v1",
				"first-k2":  "first-v2",
				"second-k1": "second-v1",
				"second-k2": "second-v2",
			},
		},
		{
			name:   "conflict",
			first:  map[string]string{"first-k1": "first-v1", "first-k2": "first-v2"},
			second: map[string]string{"first-k2": "second-v1", "second-k2": "second-v2"},
			want: map[string]string{
				"first-k1":  "first-v1",
				"first-k2":  "first-v2",
				"second-k2": "second-v2",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.EqualValues(t, tt.want, mergeMaps(tt.first, tt.second))
		})
	}
}

func assertTagInfoEqual(t *testing.T, expected *TagInfo, item *TagInfo) bool {
	t.Helper()
	sort.Strings(expected.LowCardTags)
	sort.Strings(item.LowCardTags)

	sort.Strings(expected.OrchestratorCardTags)
	sort.Strings(item.OrchestratorCardTags)

	sort.Strings(expected.HighCardTags)
	sort.Strings(item.HighCardTags)

	sort.Strings(expected.StandardTags)
	sort.Strings(item.StandardTags)

	return assert.Equal(t, expected, item)
}

func assertTagInfoListEqual(t *testing.T, expectedUpdates []*TagInfo, updates []*TagInfo) {
	t.Helper()
	assert.Equal(t, len(expectedUpdates), len(updates))
	for i := 0; i < len(expectedUpdates); i++ {
		assertTagInfoEqual(t, expectedUpdates[i], updates[i])
	}
}
