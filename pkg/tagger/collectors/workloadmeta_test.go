// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collectors

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	"github.com/stretchr/testify/assert"
)

const (
	sourcePod       = "workloadmeta-kubernetes_pod"
	sourceContainer = "workloadmeta-container"
)

type store struct {
	containers map[string]*workloadmeta.Container
}

func (s *store) Subscribe(string, *workloadmeta.Filter) chan workloadmeta.EventBundle {
	return nil
}

func (s *store) Unsubscribe(chan workloadmeta.EventBundle) {}

func (s *store) GetContainer(id string) (*workloadmeta.Container, error) {
	c, ok := s.containers[id]
	if !ok {
		return c, errors.NewNotFound(id)
	}

	return c, nil
}

func TestHandleKubePod(t *testing.T) {
	const (
		fullyFleshedContainerID = "foobarquux"
		noEnvContainerID        = "foobarbaz"
		containerName           = "agent"
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

	store := &store{
		containers: map[string]*workloadmeta.Container{
			fullyFleshedContainerID: {
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindContainer,
					ID:   fullyFleshedContainerID,
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name: containerName,
				},
				Image: workloadmeta.ContainerImage{
					ID:        "datadog/agent@sha256:a63d3f66fb2f69d955d4f2ca0b229385537a77872ffc04290acae65aed5317d2",
					RawName:   "datadog/agent@sha256:a63d3f66fb2f69d955d4f2ca0b229385537a77872ffc04290acae65aed5317d2",
					Name:      "datadog/agent",
					ShortName: "agent",
					Tag:       "latest",
				},
				EnvVars: map[string]string{
					"DD_ENV":     env,
					"DD_SERVICE": svc,
					"DD_VERSION": version,
				},
			},
			noEnvContainerID: {
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindContainer,
					ID:   noEnvContainerID,
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name: containerName,
				},
			},
		},
	}

	tests := []struct {
		name              string
		labelsAsTags      map[string]string
		annotationsAsTags map[string]string
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

				// Owner tags
				Owners: []workloadmeta.KubernetesPodOwner{
					{
						Kind: kubernetes.DeploymentKind,
						Name: svc,
					},
				},

				// PVC tags
				PersistentVolumeClaimNames: []string{"pvc-0"},

				// Phase tags
				Phase: "Running",

				// Container tags
				Containers: []string{},
			},
			expected: []*TagInfo{
				{
					Source: sourcePod,
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
				Containers: []string{fullyFleshedContainerID},
			},
			expected: []*TagInfo{
				{
					Source:       sourcePod,
					Entity:       podTaggerEntityID,
					HighCardTags: []string{},
					OrchestratorCardTags: []string{
						fmt.Sprintf("pod_name:%s", podName),
					},
					LowCardTags: append([]string{
						fmt.Sprintf("kube_namespace:%s", podNamespace),
					}),
					StandardTags: []string{},
				},
				{
					Source: sourcePod,
					Entity: fullyFleshedContainerTaggerEntityID,
					HighCardTags: []string{
						fmt.Sprintf("container_id:%s", fullyFleshedContainerID),
						fmt.Sprintf("display_container_name:%s_%s", containerName, podName),
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
				Containers: []string{noEnvContainerID},
			},
			expected: []*TagInfo{
				{
					Source:       sourcePod,
					Entity:       podTaggerEntityID,
					HighCardTags: []string{},
					OrchestratorCardTags: []string{
						fmt.Sprintf("pod_name:%s", podName),
					},
					LowCardTags: append([]string{
						fmt.Sprintf("kube_namespace:%s", podNamespace),
					}),
					StandardTags: []string{},
				},
				{
					Source: sourcePod,
					Entity: noEnvContainerTaggerEntityID,
					HighCardTags: []string{
						fmt.Sprintf("container_id:%s", noEnvContainerID),
						fmt.Sprintf("display_container_name:%s_%s", containerName, podName),
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
					Source:       sourcePod,
					Entity:       podTaggerEntityID,
					HighCardTags: []string{},
					OrchestratorCardTags: []string{
						fmt.Sprintf("pod_name:%s", podName),
						"oshift_deployment:gitlab-ce-1",
					},
					LowCardTags: append([]string{
						fmt.Sprintf("kube_namespace:%s", podNamespace),
						"oshift_deployment_config:gitlab-ce",
					}),
					StandardTags: []string{},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			collector := &WorkloadMetaCollector{
				store: store,
			}
			collector.initPodMetaAsTags(tt.labelsAsTags, tt.annotationsAsTags)

			actual := collector.handleKubePod(workloadmeta.Event{
				Type:   workloadmeta.EventTypeSet,
				Entity: &tt.pod,
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
					Source: sourceContainer,
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
					Source: sourceContainer,
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
					Source: sourceContainer,
					Entity: taggerEntityID,
					HighCardTags: []string{
						fmt.Sprintf("container_name:%s", containerName),
						fmt.Sprintf("container_id:%s", entityID.ID),
						"app_name:datadog-agent",
					},
					OrchestratorCardTags: []string{},
					LowCardTags: append([]string{
						"owner_team:container-integrations",
					}),
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
				},
			},
			expected: []*TagInfo{
				{
					Source: sourceContainer,
					Entity: taggerEntityID,
					HighCardTags: []string{
						fmt.Sprintf("container_name:%s", containerName),
						fmt.Sprintf("container_id:%s", entityID.ID),
					},
					OrchestratorCardTags: []string{},
					LowCardTags: append([]string{
						"nomad_task:test-task",
						"nomad_job:test-job",
						"nomad_group:test-group",
					}),
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
					Source: sourceContainer,
					Entity: taggerEntityID,
					HighCardTags: []string{
						fmt.Sprintf("container_name:%s", containerName),
						fmt.Sprintf("container_id:%s", entityID.ID),
					},
					OrchestratorCardTags: []string{
						"mesos_task:system_dd-agent.dcc75b42-4b87-11e7-9a62-70b3d5800001",
					},
					LowCardTags: append([]string{
						"chronos_job:app1_process-orders",
						"chronos_job_owner:qa",
						"marathon_app:/system/dd-agent",
					}),
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
					Source: sourceContainer,
					Entity: taggerEntityID,
					HighCardTags: []string{
						fmt.Sprintf("container_name:%s", containerName),
						fmt.Sprintf("container_id:%s", entityID.ID),
						"rancher_container:testAD-redis-1",
					},
					OrchestratorCardTags: []string{},
					LowCardTags: append([]string{
						"rancher_service:testAD/redis",
						"rancher_stack:testAD",
					}),
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
					Source: sourceContainer,
					Entity: taggerEntityID,
					HighCardTags: []string{
						fmt.Sprintf("container_name:%s", containerName),
						fmt.Sprintf("container_id:%s", entityID.ID),
					},
					OrchestratorCardTags: []string{},
					LowCardTags: append([]string{
						"swarm_namespace:default",
						"swarm_service:helloworld",
					}),
					StandardTags: []string{},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			collector := &WorkloadMetaCollector{}
			collector.initContainerMetaAsTags(tt.labelsAsTags, tt.envAsTags)

			actual := collector.handleContainer(workloadmeta.Event{
				Type:   workloadmeta.EventTypeSet,
				Entity: &tt.container,
			})

			assertTagInfoListEqual(t, tt.expected, actual)
		})
	}
}

func TestParseJSONValue(t *testing.T) {
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
