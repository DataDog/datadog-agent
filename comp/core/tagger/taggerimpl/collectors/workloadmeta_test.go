// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collectors

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/tagger/common"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taglist"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

func TestHandleKubePod(t *testing.T) {
	const (
		fullyFleshedContainerID = "foobarquux"
		otelEnvContainerID      = "otelcontainer"
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

	podTaggerEntityID := types.NewEntityID(types.KubernetesPodUID, podEntityID.ID)
	fullyFleshedContainerTaggerEntityID := types.NewEntityID(types.ContainerID, fullyFleshedContainerID)
	noEnvContainerTaggerEntityID := types.NewEntityID(types.ContainerID, noEnvContainerID)

	image := workloadmeta.ContainerImage{
		ID:        "datadog/agent@sha256:a63d3f66fb2f69d955d4f2ca0b229385537a77872ffc04290acae65aed5317d2",
		RawName:   "datadog/agent@sha256:a63d3f66fb2f69d955d4f2ca0b229385537a77872ffc04290acae65aed5317d2",
		Name:      "datadog/agent",
		ShortName: "agent",
		Tag:       "latest",
	}

	store := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		fx.Provide(func() log.Component { return logmock.New(t) }),
		config.MockModule(),
		fx.Supply(context.Background()),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
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
			ID:   otelEnvContainerID,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: runtimeContainerName,
		},
		Image: image,
		EnvVars: map[string]string{
			"OTEL_SERVICE_NAME":        svc,
			"OTEL_RESOURCE_ATTRIBUTES": fmt.Sprintf("service.name=%s,service.version=%s,deployment.environment=%s", svc, version, env),
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
		name                          string
		staticTags                    map[string]string
		k8sResourcesAnnotationsAsTags map[string]map[string]string
		k8sResourcesLabelsAsTags      map[string]map[string]string
		pod                           workloadmeta.KubernetesPod
		expected                      []*types.TagInfo
	}{
		{
			name: "fully formed pod (no containers)",
			k8sResourcesAnnotationsAsTags: map[string]map[string]string{
				"pods": {
					"ns_tier":   "ns_tier",
					"ns_custom": "custom_generic_annotation",
					"gitcommit": "+gitcommit",
					"component": "component",
				},
				"namespaces": {
					"ns_tier":            "ns_tier",
					"namespace_security": "ns_security",
				},
			},
			k8sResourcesLabelsAsTags: map[string]map[string]string{
				"pods": {
					"ns_env":    "ns_env",
					"ns_custom": "custom_generic_label",
					"ownerteam": "team",
					"tier":      "tier",
				},
				"namespaces": {
					"ns_env":       "ns_env",
					"ns_ownerteam": "ns_team",
				},
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
						"ns_custom": "gee",

						// Custom tags from map
						"ad.datadoghq.com/tags": `{"pod_template_version":"1.0.0"}`,
					},
					Labels: map[string]string{
						// Labels as tags
						"OwnerTeam":         "container-integrations",
						"tier":              "node",
						"pod-template-hash": "490794276",
						"ns_custom":         "zoo",

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
					"ns_ownerteam": "containers",
					"foo":          "bar",
				},

				// NS annotations as tags
				NamespaceAnnotations: map[string]string{
					"ns_tier":            "some_tier",
					"namespace_security": "critical",
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

				// Runtime Class tag
				RuntimeClass: "myclass",

				// Phase tags
				Phase: "Running",

				PriorityClass: "high-priority",
			},
			expected: []*types.TagInfo{
				{
					Source:   podSource,
					EntityID: podTaggerEntityID,
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
						"kube_priority_class:high-priority",
						"kube_service:service1",
						"kube_service:service2",
						"kube_qos:guaranteed",
						"kube_runtime_class:myclass",
						"ns_team:containers",
						"ns_env:dev",
						"ns_tier:some_tier",
						"ns_security:critical",
						"pod_phase:running",
						"pod_template_version:1.0.0",
						"team:container-integrations",
						"tier:node",
						"custom_generic_label:zoo",
						"custom_generic_annotation:gee",
					}, standardTags...),
					StandardTags: standardTags,
				},
			},
		},
		{
			name: "persistent volume claim tags activated",
			pod: workloadmeta.KubernetesPod{
				EntityID: podEntityID,
				EntityMeta: workloadmeta.EntityMeta{
					Name:      podName,
					Namespace: podNamespace,
				},
				Containers: []workloadmeta.OrchestratorContainer{
					{
						ID:    noEnvContainerID,
						Name:  containerName,
						Image: image,
					},
				},
				Owners: []workloadmeta.KubernetesPodOwner{
					{
						Kind: kubernetes.StatefulSetKind,
					},
				},
				// PVC tags
				PersistentVolumeClaimNames: []string{"pvc-0"},
			},
			expected: []*types.TagInfo{
				{
					Source:       podSource,
					EntityID:     podTaggerEntityID,
					HighCardTags: []string{},
					OrchestratorCardTags: []string{
						fmt.Sprintf("pod_name:%s", podName),
					},
					LowCardTags: []string{
						fmt.Sprintf("kube_namespace:%s", podNamespace),
						"kube_ownerref_kind:statefulset",
						"persistentvolumeclaim:pvc-0",
					},
					StandardTags: []string{},
				},
				{
					Source:   podSource,
					EntityID: noEnvContainerTaggerEntityID,
					HighCardTags: []string{
						fmt.Sprintf("container_id:%s", noEnvContainerID),
						fmt.Sprintf("display_container_name:%s_%s", runtimeContainerName, podName),
					},
					OrchestratorCardTags: []string{
						fmt.Sprintf("pod_name:%s", podName),
					},
					LowCardTags: []string{
						fmt.Sprintf("kube_namespace:%s", podNamespace),
						fmt.Sprintf("kube_container_name:%s", containerName),
						"image_id:datadog/agent@sha256:a63d3f66fb2f69d955d4f2ca0b229385537a77872ffc04290acae65aed5317d2",
						"image_name:datadog/agent",
						"image_tag:latest",
						"short_image:agent",
						"kube_ownerref_kind:statefulset",
						"persistentvolumeclaim:pvc-0",
					},
					StandardTags: []string{},
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
			expected: []*types.TagInfo{
				{
					Source:       podSource,
					EntityID:     podTaggerEntityID,
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
					Source:   podSource,
					EntityID: fullyFleshedContainerTaggerEntityID,
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
			name: "pod with fully formed container, standard tags from env with opentelemetry sdk",
			pod: workloadmeta.KubernetesPod{
				EntityID: podEntityID,
				EntityMeta: workloadmeta.EntityMeta{
					Name:      podName,
					Namespace: podNamespace,
				},
				Containers: []workloadmeta.OrchestratorContainer{
					{
						ID:    otelEnvContainerID,
						Name:  containerName,
						Image: image,
					},
				},
			},
			expected: []*types.TagInfo{
				{
					Source:       podSource,
					EntityID:     podTaggerEntityID,
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
					Source:   podSource,
					EntityID: types.NewEntityID(types.ContainerID, otelEnvContainerID),
					HighCardTags: []string{
						fmt.Sprintf("container_id:%s", otelEnvContainerID),
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
			expected: []*types.TagInfo{
				{
					Source:       podSource,
					EntityID:     podTaggerEntityID,
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
					Source:   podSource,
					EntityID: noEnvContainerTaggerEntityID,
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
			expected: []*types.TagInfo{
				{
					Source:       podSource,
					EntityID:     podTaggerEntityID,
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
			expected: []*types.TagInfo{
				{
					Source:       podSource,
					EntityID:     podTaggerEntityID,
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
			name: "pod owned by daemonset",
			pod: workloadmeta.KubernetesPod{
				EntityID: podEntityID,
				EntityMeta: workloadmeta.EntityMeta{
					Name:      podName,
					Namespace: podNamespace,
				},
				Owners: []workloadmeta.KubernetesPodOwner{
					{
						Kind: kubernetes.DaemonSetKind,
						Name: "owner_name",
					},
				},
			},
			expected: []*types.TagInfo{
				{
					Source:       podSource,
					EntityID:     podTaggerEntityID,
					HighCardTags: []string{},
					OrchestratorCardTags: []string{
						fmt.Sprintf("pod_name:%s", podName),
						"kube_ownerref_name:owner_name",
					},
					LowCardTags: []string{
						fmt.Sprintf("kube_namespace:%s", podNamespace),
						"kube_ownerref_kind:daemonset",
						"kube_daemon_set:owner_name",
					},
					StandardTags: []string{},
				},
			},
		},
		{
			name: "pod owned by replication controller",
			pod: workloadmeta.KubernetesPod{
				EntityID: podEntityID,
				EntityMeta: workloadmeta.EntityMeta{
					Name:      podName,
					Namespace: podNamespace,
				},
				Owners: []workloadmeta.KubernetesPodOwner{
					{
						Kind: kubernetes.ReplicationControllerKind,
						Name: "owner_name",
					},
				},
			},
			expected: []*types.TagInfo{
				{
					Source:       podSource,
					EntityID:     podTaggerEntityID,
					HighCardTags: []string{},
					OrchestratorCardTags: []string{
						fmt.Sprintf("pod_name:%s", podName),
						"kube_ownerref_name:owner_name",
					},
					LowCardTags: []string{
						fmt.Sprintf("kube_namespace:%s", podNamespace),
						"kube_ownerref_kind:replicationcontroller",
						"kube_replication_controller:owner_name",
					},
					StandardTags: []string{},
				},
			},
		},
		{
			name: "pod owned by statefulset with persistent volume claim",
			pod: workloadmeta.KubernetesPod{
				EntityID: podEntityID,
				EntityMeta: workloadmeta.EntityMeta{
					Name:      podName,
					Namespace: podNamespace,
				},
				Owners: []workloadmeta.KubernetesPodOwner{
					{
						Kind: kubernetes.StatefulSetKind,
						Name: "owner_name",
					},
				},
				PersistentVolumeClaimNames: []string{
					"pvc-0",
				},
			},
			expected: []*types.TagInfo{
				{
					Source:       podSource,
					EntityID:     podTaggerEntityID,
					HighCardTags: []string{},
					OrchestratorCardTags: []string{
						fmt.Sprintf("pod_name:%s", podName),
						"kube_ownerref_name:owner_name",
					},
					LowCardTags: []string{
						fmt.Sprintf("kube_namespace:%s", podNamespace),
						"kube_ownerref_kind:statefulset",
						"kube_stateful_set:owner_name",
						"persistentvolumeclaim:pvc-0",
					},
					StandardTags: []string{},
				},
			},
		},
		{
			name: "pod owned by job",
			pod: workloadmeta.KubernetesPod{
				EntityID: podEntityID,
				EntityMeta: workloadmeta.EntityMeta{
					Name:      podName,
					Namespace: podNamespace,
				},
				Owners: []workloadmeta.KubernetesPodOwner{
					{
						Kind: kubernetes.JobKind,
						Name: "owner_name",
					},
				},
			},
			expected: []*types.TagInfo{
				{
					Source:       podSource,
					EntityID:     podTaggerEntityID,
					HighCardTags: []string{},
					OrchestratorCardTags: []string{
						fmt.Sprintf("pod_name:%s", podName),
						"kube_ownerref_name:owner_name",
					},
					LowCardTags: []string{
						fmt.Sprintf("kube_namespace:%s", podNamespace),
						"kube_ownerref_kind:job",
						"kube_job:owner_name",
					},
					StandardTags: []string{},
				},
			},
		},
		{
			name: "pod owned by cronjob",
			pod: workloadmeta.KubernetesPod{
				EntityID: podEntityID,
				EntityMeta: workloadmeta.EntityMeta{
					Name:      podName,
					Namespace: podNamespace,
				},
				Owners: []workloadmeta.KubernetesPodOwner{
					{
						Kind: kubernetes.JobKind,
						// job owned by "some_cronjob".
						// Notice that, to make this test work, the job name
						// included after the "-" separator needs to be valid
						// according to the ParseCronJobForJob function.
						Name: "some_cronjob-123",
					},
				},
			},
			expected: []*types.TagInfo{
				{
					Source:       podSource,
					EntityID:     podTaggerEntityID,
					HighCardTags: []string{},
					OrchestratorCardTags: []string{
						fmt.Sprintf("pod_name:%s", podName),
						"kube_ownerref_name:some_cronjob-123",
						"kube_job:some_cronjob-123",
					},
					LowCardTags: []string{
						fmt.Sprintf("kube_namespace:%s", podNamespace),
						"kube_ownerref_kind:job",
						"kube_cronjob:some_cronjob",
					},
					StandardTags: []string{},
				},
			},
		},
		{
			name: "pod owned by replicaset without deployment",
			pod: workloadmeta.KubernetesPod{
				EntityID: podEntityID,
				EntityMeta: workloadmeta.EntityMeta{
					Name:      podName,
					Namespace: podNamespace,
				},
				Owners: []workloadmeta.KubernetesPodOwner{
					{
						Kind: kubernetes.ReplicaSetKind,
						Name: "owner_name",
					},
				},
			},
			expected: []*types.TagInfo{
				{
					Source:       podSource,
					EntityID:     podTaggerEntityID,
					HighCardTags: []string{},
					OrchestratorCardTags: []string{
						fmt.Sprintf("pod_name:%s", podName),
						"kube_ownerref_name:owner_name",
					},
					LowCardTags: []string{
						fmt.Sprintf("kube_namespace:%s", podNamespace),
						"kube_ownerref_kind:replicaset",
						"kube_replica_set:owner_name",
					},
					StandardTags: []string{},
				},
			},
		},
		{
			name: "pod owned by replicaset that belongs to a deployment",
			pod: workloadmeta.KubernetesPod{
				EntityID: podEntityID,
				EntityMeta: workloadmeta.EntityMeta{
					Name:      podName,
					Namespace: podNamespace,
				},
				Owners: []workloadmeta.KubernetesPodOwner{
					{
						Kind: kubernetes.ReplicaSetKind,
						// replicaset owned by "some_deployment"
						// Notice that, to make this test work, the replicaset
						// name included after the "-" separator needs to be
						// valid according to the ParseDeploymentForReplicaSet
						// function.
						Name: "some_deployment-bcd2",
					},
				},
			},
			expected: []*types.TagInfo{
				{
					Source:       podSource,
					EntityID:     podTaggerEntityID,
					HighCardTags: []string{},
					OrchestratorCardTags: []string{
						fmt.Sprintf("pod_name:%s", podName),
						"kube_ownerref_name:some_deployment-bcd2",
					},
					LowCardTags: []string{
						fmt.Sprintf("kube_namespace:%s", podNamespace),
						"kube_ownerref_kind:replicaset",
						"kube_replica_set:some_deployment-bcd2",
						"kube_deployment:some_deployment",
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
			expected: []*types.TagInfo{
				{
					Source:       podSource,
					EntityID:     podTaggerEntityID,
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
			expected: []*types.TagInfo{
				{
					Source:       podSource,
					EntityID:     podTaggerEntityID,
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
		{
			name: "pod with containers requesting gpu resources",
			pod: workloadmeta.KubernetesPod{
				EntityID: podEntityID,
				EntityMeta: workloadmeta.EntityMeta{
					Name:      podName,
					Namespace: podNamespace,
				},
				GPUVendorList: []string{"nvidia"},
				Containers: []workloadmeta.OrchestratorContainer{
					{
						ID:    fullyFleshedContainerID,
						Name:  containerName,
						Image: image,
					},
				},
			},
			expected: []*types.TagInfo{
				{
					Source:       podSource,
					EntityID:     podTaggerEntityID,
					HighCardTags: []string{},
					OrchestratorCardTags: []string{
						fmt.Sprintf("pod_name:%s", podName),
					},
					LowCardTags: []string{
						fmt.Sprintf("kube_namespace:%s", podNamespace),
						"gpu_vendor:nvidia",
					},
					StandardTags: []string{},
				},
				{
					Source:   podSource,
					EntityID: fullyFleshedContainerTaggerEntityID,
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
						"gpu_vendor:nvidia",
					}, standardTags...),
					StandardTags: standardTags,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := configmock.New(t)
			collector := NewWorkloadMetaCollector(context.Background(), cfg, store, nil)
			collector.staticTags = tt.staticTags
			collector.initK8sResourcesMetaAsTags(tt.k8sResourcesLabelsAsTags, tt.k8sResourcesAnnotationsAsTags)

			actual := collector.handleKubePod(workloadmeta.Event{
				Type:   workloadmeta.EventTypeSet,
				Entity: &tt.pod,
			})

			assertTagInfoListEqual(t, tt.expected, actual)
		})
	}
}

func TestHandleKubePodWithoutPvcAsTags(t *testing.T) {
	const (
		noEnvContainerID     = "foobarbaz"
		containerName        = "agent"
		runtimeContainerName = "k8s_datadog-agent_agent"
		podName              = "datadog-agent-foobar"
		podNamespace         = "default"
	)

	podEntityID := workloadmeta.EntityID{
		Kind: workloadmeta.KindKubernetesPod,
		ID:   "foobar",
	}

	podTaggerEntityID := types.NewEntityID(types.KubernetesPodUID, podEntityID.ID)

	image := workloadmeta.ContainerImage{
		ID:        "datadog/agent@sha256:a63d3f66fb2f69d955d4f2ca0b229385537a77872ffc04290acae65aed5317d2",
		RawName:   "datadog/agent@sha256:a63d3f66fb2f69d955d4f2ca0b229385537a77872ffc04290acae65aed5317d2",
		Name:      "datadog/agent",
		ShortName: "agent",
		Tag:       "latest",
	}

	store := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		fx.Provide(func() log.Component { return logmock.New(t) }),
		config.MockModule(),
		fx.Supply(context.Background()),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	), fx.Replace(config.MockParams{Overrides: map[string]any{
		"kubernetes_persistent_volume_claims_as_tags": false,
	}}))
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
		name                string
		staticTags          map[string]string
		labelsAsTags        map[string]string
		annotationsAsTags   map[string]string
		nsLabelsAsTags      map[string]string
		nsAnnotationsAsTags map[string]string
		pod                 workloadmeta.KubernetesPod
		expected            []*types.TagInfo
	}{
		{
			name: "persistent volume claim tags deactivated",
			pod: workloadmeta.KubernetesPod{
				EntityID: podEntityID,
				EntityMeta: workloadmeta.EntityMeta{
					Name:      podName,
					Namespace: podNamespace,
				},
				Containers: []workloadmeta.OrchestratorContainer{
					{
						ID:    noEnvContainerID,
						Name:  containerName,
						Image: image,
					},
				},
				Owners: []workloadmeta.KubernetesPodOwner{
					{
						Kind: kubernetes.StatefulSetKind,
					},
				},
				// PVC tags
				PersistentVolumeClaimNames: []string{"pvc-0"},
			},
			expected: []*types.TagInfo{
				{
					Source:       podSource,
					EntityID:     podTaggerEntityID,
					HighCardTags: []string{},
					OrchestratorCardTags: []string{
						fmt.Sprintf("pod_name:%s", podName),
					},
					LowCardTags: []string{
						fmt.Sprintf("kube_namespace:%s", podNamespace),
						"kube_ownerref_kind:statefulset",
					},
					StandardTags: []string{},
				},
				{
					Source:   podSource,
					EntityID: types.NewEntityID(types.ContainerID, noEnvContainerID),
					HighCardTags: []string{
						fmt.Sprintf("container_id:%s", noEnvContainerID),
						fmt.Sprintf("display_container_name:%s_%s", runtimeContainerName, podName),
					},
					OrchestratorCardTags: []string{
						fmt.Sprintf("pod_name:%s", podName),
					},
					LowCardTags: []string{
						fmt.Sprintf("kube_namespace:%s", podNamespace),
						fmt.Sprintf("kube_container_name:%s", containerName),
						"image_id:datadog/agent@sha256:a63d3f66fb2f69d955d4f2ca0b229385537a77872ffc04290acae65aed5317d2",
						"image_name:datadog/agent",
						"image_tag:latest",
						"short_image:agent",
						"kube_ownerref_kind:statefulset",
					},
					StandardTags: []string{},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := configmock.New(t)
			cfg.SetWithoutSource("kubernetes_persistent_volume_claims_as_tags", false)
			collector := NewWorkloadMetaCollector(context.Background(), cfg, store, nil)
			collector.staticTags = tt.staticTags

			actual := collector.handleKubePod(workloadmeta.Event{
				Type:   workloadmeta.EventTypeSet,
				Entity: &tt.pod,
			})

			assertTagInfoListEqual(t, tt.expected, actual)
		})
	}
}

func TestHandleKubePodNoContainerName(t *testing.T) {
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

	podTaggerEntityID := types.NewEntityID(types.KubernetesPodUID, podEntityID.ID)
	fullyFleshedContainerTaggerEntityID := types.NewEntityID(types.ContainerID, fullyFleshedContainerID)

	image := workloadmeta.ContainerImage{
		ID:        "datadog/agent@sha256:a63d3f66fb2f69d955d4f2ca0b229385537a77872ffc04290acae65aed5317d2",
		RawName:   "datadog/agent@sha256:a63d3f66fb2f69d955d4f2ca0b229385537a77872ffc04290acae65aed5317d2",
		Name:      "datadog/agent",
		ShortName: "agent",
		Tag:       "latest",
	}

	store := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		fx.Provide(func() log.Component { return logmock.New(t) }),
		config.MockModule(),
		fx.Supply(context.Background()),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	store.Set(&workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   fullyFleshedContainerID,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: "",
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
		name                string
		staticTags          map[string]string
		labelsAsTags        map[string]string
		annotationsAsTags   map[string]string
		nsLabelsAsTags      map[string]string
		nsAnnotationsAsTags map[string]string
		pod                 workloadmeta.KubernetesPod
		expected            []*types.TagInfo
	}{
		{
			name: "pod with no container name",
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
			expected: []*types.TagInfo{
				{
					Source:       podSource,
					EntityID:     podTaggerEntityID,
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
					Source:   podSource,
					EntityID: fullyFleshedContainerTaggerEntityID,
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := configmock.New(t)
			collector := NewWorkloadMetaCollector(context.Background(), cfg, store, nil)
			collector.staticTags = tt.staticTags

			actual := collector.handleKubePod(workloadmeta.Event{
				Type:   workloadmeta.EventTypeSet,
				Entity: &tt.pod,
			})

			assertTagInfoListEqual(t, tt.expected, actual)
		})
	}
}

func TestHandleKubeMetadata(t *testing.T) {
	const namespace = "foobar"

	kubeMetadataEntityID := workloadmeta.EntityID{
		Kind: workloadmeta.KindKubernetesMetadata,
		ID:   fmt.Sprintf("namespaces//%s", namespace),
	}

	store := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		fx.Provide(func() log.Component { return logmock.New(t) }),
		config.MockModule(),
		fx.Supply(context.Background()),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	store.Set(&workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesMetadata,
			ID:   fmt.Sprintf("namespaces//%s", namespace),
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: namespace,
		},
	})

	tests := []struct {
		name                          string
		k8sResourcesAnnotationsAsTags map[string]map[string]string
		k8sResourcesLabelsAsTags      map[string]map[string]string
		kubeMetadata                  workloadmeta.KubernetesMetadata
		expected                      []*types.TagInfo
	}{
		{
			name: "namespace with labels and annotations as tags",
			k8sResourcesAnnotationsAsTags: map[string]map[string]string{
				"namespaces": {
					"ns_tier":            "ns_tier",
					"ns_custom":          "custom_generic_annotation",
					"namespace_security": "ns_security",
				},
			},
			k8sResourcesLabelsAsTags: map[string]map[string]string{
				"namespaces": {
					"ns_env":       "ns_env",
					"ns_custom":    "custom_generic_label",
					"ns_ownerteam": "ns_team",
				},
			},
			kubeMetadata: workloadmeta.KubernetesMetadata{
				EntityID: kubeMetadataEntityID,
				EntityMeta: workloadmeta.EntityMeta{
					Name: namespace,
					Labels: map[string]string{
						"a": "dev",
					},
					Annotations: map[string]string{
						"b": "some_tier",
					},
				},
				GVR: &schema.GroupVersionResource{
					Version:  "v1",
					Resource: "namespaces",
				},
			},
			expected: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			cfg := configmock.New(t)
			collector := NewWorkloadMetaCollector(context.Background(), cfg, store, nil)

			collector.initK8sResourcesMetaAsTags(test.k8sResourcesLabelsAsTags, test.k8sResourcesAnnotationsAsTags)

			actual := collector.handleKubeMetadata(workloadmeta.Event{
				Type:   workloadmeta.EventTypeSet,
				Entity: &test.kubeMetadata,
			})

			assertTagInfoListEqual(tt, test.expected, actual)
		})
	}
}

func TestHandleKubeDeployment(t *testing.T) {
	const deploymentName = "fooapp"

	kubeMetadataID := string(util.GenerateKubeMetadataEntityID("apps", "deployments", "default", deploymentName))

	kubeMetadataEntityID := workloadmeta.EntityID{
		Kind: workloadmeta.KindKubernetesMetadata,
		ID:   kubeMetadataID,
	}

	taggerEntityID := types.NewEntityID(types.KubernetesMetadata, kubeMetadataEntityID.ID)

	store := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		fx.Provide(func() log.Component { return logmock.New(t) }),
		config.MockModule(),
		fx.Supply(context.Background()),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	store.Set(&workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesMetadata,
			ID:   kubeMetadataID,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:      deploymentName,
			Namespace: "default",
		},
	})

	tests := []struct {
		name                          string
		k8sResourcesAnnotationsAsTags map[string]map[string]string
		k8sResourcesLabelsAsTags      map[string]map[string]string
		kubeMetadata                  workloadmeta.KubernetesMetadata
		expected                      []*types.TagInfo
	}{
		{
			name: "deployment with no matching labels/annotations for annotations/labels as tags. should return nil to avoid empty tagger entity",
			k8sResourcesAnnotationsAsTags: map[string]map[string]string{
				"deployments.apps": {
					"depl_tier":   "depl_tier",
					"depl_custom": "custom_generic_annotation",
				},
			},
			k8sResourcesLabelsAsTags: map[string]map[string]string{
				"deployments.apps": {
					"depl_env":    "depl_env",
					"depl_custom": "custom_generic_label",
				},
			},
			kubeMetadata: workloadmeta.KubernetesMetadata{
				EntityID: kubeMetadataEntityID,
				EntityMeta: workloadmeta.EntityMeta{
					Name: deploymentName,
					Labels: map[string]string{
						"a": "dev",
					},
					Annotations: map[string]string{
						"b": "some_tier",
					},
				},
				GVR: &schema.GroupVersionResource{
					Version:  "v1",
					Group:    "apps",
					Resource: "deployments",
				},
			},
			expected: nil,
		},
		{
			name: "deployment with generic annotations/labels as tags",
			k8sResourcesAnnotationsAsTags: map[string]map[string]string{
				"deployments.apps": {
					"depl_tier":   "depl_tier",
					"depl_custom": "custom_generic_annotation",
				},
			},
			k8sResourcesLabelsAsTags: map[string]map[string]string{
				"deployments.apps": {
					"depl_env":    "depl_env",
					"depl_custom": "custom_generic_label",
				},
			},
			kubeMetadata: workloadmeta.KubernetesMetadata{
				EntityID: kubeMetadataEntityID,
				EntityMeta: workloadmeta.EntityMeta{
					Name: deploymentName,
					Labels: map[string]string{
						"depl_env":       "dev",
						"depl_ownerteam": "containers",
						"foo":            "bar",
						"depl_custom":    "zoo",
					},
					Annotations: map[string]string{
						"depl_tier":     "some_tier",
						"depl_security": "critical",
						"depl_custom":   "gee",
					},
				},
				GVR: &schema.GroupVersionResource{
					Version:  "v1",
					Group:    "apps",
					Resource: "deployments",
				},
			},
			expected: []*types.TagInfo{
				{
					Source:               kubeMetadataSource,
					EntityID:             taggerEntityID,
					HighCardTags:         []string{},
					OrchestratorCardTags: []string{},
					LowCardTags: []string{
						"depl_env:dev",
						"depl_tier:some_tier",
						"custom_generic_label:zoo",
						"custom_generic_annotation:gee",
					},
					StandardTags: []string{},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			cfg := configmock.New(t)
			collector := NewWorkloadMetaCollector(context.Background(), cfg, store, nil)

			collector.initK8sResourcesMetaAsTags(test.k8sResourcesLabelsAsTags, test.k8sResourcesAnnotationsAsTags)

			actual := collector.handleKubeMetadata(workloadmeta.Event{
				Type:   workloadmeta.EventTypeSet,
				Entity: &test.kubeMetadata,
			})

			assertTagInfoListEqual(tt, test.expected, actual)
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

	taggerEntityID := types.NewEntityID(types.ContainerID, containerID)

	store := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		fx.Provide(func() log.Component { return logmock.New(t) }),
		config.MockModule(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	), fx.Replace(config.MockParams{Overrides: map[string]any{
		"ecs_collect_resource_tags_ec2": true,
	}}))

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
		expected []*types.TagInfo
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
				ClusterName:  "ecs-cluster",
				Family:       "datadog-agent",
				Version:      "1",
				AWSAccountID: 1234567891234,
				LaunchType:   workloadmeta.ECSLaunchTypeEC2,
				Containers: []workloadmeta.OrchestratorContainer{
					{
						ID:   containerID,
						Name: containerName,
					},
				},
				ServiceName: "datadog-agent-service",
			},
			expected: []*types.TagInfo{
				{
					Source:       taskSource,
					EntityID:     taggerEntityID,
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
						"ecs_service:datadog-agent-service",
						"aws_account:1234567891234",
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
				Region:           "us-east-1",
				AWSAccountID:     1234567891234,
			},
			expected: []*types.TagInfo{
				{
					Source:       taskSource,
					EntityID:     taggerEntityID,
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
						"region:us-east-1",
						"aws_account:1234567891234",
					},
					StandardTags: []string{},
				},
				{
					Source:       taskSource,
					EntityID:     common.GetGlobalEntityID(),
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
						"region:us-east-1",
						"aws_account:1234567891234",
					},
					StandardTags: []string{},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := configmock.New(t)
			cfg.SetWithoutSource("ecs_collect_resource_tags_ec2", true)
			collector := NewWorkloadMetaCollector(context.Background(), cfg, store, nil)

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
		repositoryURL = "https://github.com/DataDog/datadog-agent"
		commitSHA     = "ce12f4c957dc5083c390205da435ebf54b9f7dac"
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

	taggerEntityID := types.NewEntityID(types.ContainerID, entityID.ID)

	tests := []struct {
		name         string
		staticTags   map[string]string
		labelsAsTags map[string]string
		envAsTags    map[string]string
		container    workloadmeta.Container
		expected     []*types.TagInfo
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
			expected: []*types.TagInfo{
				{
					Source:   containerSource,
					EntityID: taggerEntityID,
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

					// source code integration
					"DD_GIT_REPOSITORY_URL": repositoryURL,
					"DD_GIT_COMMIT_SHA":     commitSHA,
				},
			},
			envAsTags: map[string]string{
				"team": "owner_team",
			},
			expected: []*types.TagInfo{
				{
					Source:   containerSource,
					EntityID: taggerEntityID,
					HighCardTags: []string{
						fmt.Sprintf("container_name:%s", containerName),
						fmt.Sprintf("container_id:%s", entityID.ID),
					},
					OrchestratorCardTags: []string{},
					LowCardTags: append([]string{
						"owner_team:container-integrations",
						fmt.Sprintf("git.repository_url:%s", repositoryURL),
						fmt.Sprintf("git.commit.sha:%s", commitSHA),
					}, standardTags...),
					StandardTags: standardTags,
				},
			},
		},
		{
			name: "tags from environment with opentelemetry sdk",
			container: workloadmeta.Container{
				EntityID: entityID,
				EntityMeta: workloadmeta.EntityMeta{
					Name: containerName,
				},
				EnvVars: map[string]string{
					// env as tags
					"TEAM": "container-integrations",
					"TIER": "node",

					// otel standard tags
					"OTEL_SERVICE_NAME":        svc,
					"OTEL_RESOURCE_ATTRIBUTES": fmt.Sprintf("service.name=%s,service.version=%s,deployment.environment=%s", svc, version, env),
				},
			},
			envAsTags: map[string]string{
				"team": "owner_team",
			},
			expected: []*types.TagInfo{
				{
					Source:   containerSource,
					EntityID: taggerEntityID,
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
			name: "tags from environment with opentelemetry sdk with whitespace",
			container: workloadmeta.Container{
				EntityID: entityID,
				EntityMeta: workloadmeta.EntityMeta{
					Name: containerName,
				},
				EnvVars: map[string]string{
					// env as tags
					"TEAM": "container-integrations",
					"TIER": "node",

					// otel standard tags
					"OTEL_SERVICE_NAME":        svc,
					"OTEL_RESOURCE_ATTRIBUTES": fmt.Sprintf("service.name= %s, service.version = %s , deployment.environment =%s", svc, version, env),
				},
			},
			envAsTags: map[string]string{
				"team": "owner_team",
			},
			expected: []*types.TagInfo{
				{
					Source:   containerSource,
					EntityID: taggerEntityID,
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
			name: "tags from environment with malformed OTEL_RESOURCE_ATTRIBUTES",
			container: workloadmeta.Container{
				EntityID: entityID,
				EntityMeta: workloadmeta.EntityMeta{
					Name: containerName,
				},
				EnvVars: map[string]string{
					// env as tags
					"TEAM": "container-integrations",
					"TIER": "node",

					// otel standard tags
					"OTEL_RESOURCE_ATTRIBUTES": fmt.Sprintf("service.name=,  =  , =%s", env),
				},
			},
			envAsTags: map[string]string{
				"team": "owner_team",
			},
			expected: []*types.TagInfo{
				{
					Source:   containerSource,
					EntityID: taggerEntityID,
					HighCardTags: []string{
						fmt.Sprintf("container_name:%s", containerName),
						fmt.Sprintf("container_id:%s", entityID.ID),
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
			expected: []*types.TagInfo{
				{
					Source:   containerSource,
					EntityID: taggerEntityID,
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
			expected: []*types.TagInfo{
				{
					Source:   containerSource,
					EntityID: taggerEntityID,
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
			expected: []*types.TagInfo{
				{
					Source:   containerSource,
					EntityID: taggerEntityID,
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
			expected: []*types.TagInfo{
				{
					Source:   containerSource,
					EntityID: taggerEntityID,
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
			expected: []*types.TagInfo{
				{
					Source:   containerSource,
					EntityID: taggerEntityID,
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
			expected: []*types.TagInfo{
				{
					Source:   containerSource,
					EntityID: taggerEntityID,
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
			expected: []*types.TagInfo{
				{
					Source:   containerSource,
					EntityID: taggerEntityID,
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
			expected: []*types.TagInfo{
				{
					Source:   containerSource,
					EntityID: taggerEntityID,
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
			expected: []*types.TagInfo{
				{
					Source:   containerSource,
					EntityID: taggerEntityID,
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
		{
			name: "gpu tags",
			container: workloadmeta.Container{
				EntityID: entityID,
				EntityMeta: workloadmeta.EntityMeta{
					Name: containerName,
				},
				Resources: workloadmeta.ContainerResources{
					GPUVendorList: []string{"nvidia"},
				},
			},
			expected: []*types.TagInfo{
				{
					Source:   containerSource,
					EntityID: taggerEntityID,
					HighCardTags: []string{
						fmt.Sprintf("container_name:%s", containerName),
						fmt.Sprintf("container_id:%s", entityID.ID),
					},
					OrchestratorCardTags: []string{},
					LowCardTags: []string{
						"gpu_vendor:nvidia",
					},
					StandardTags: []string{},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := configmock.New(t)
			collector := NewWorkloadMetaCollector(context.Background(), cfg, nil, nil)
			collector.staticTags = tt.staticTags

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

	taggerEntityID := types.NewEntityID(types.ContainerImageMetadata, entityID.ID)

	tests := []struct {
		name     string
		image    workloadmeta.ContainerImageMetadata
		expected []*types.TagInfo
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
			expected: []*types.TagInfo{
				{
					Source:               containerImageSource,
					EntityID:             taggerEntityID,
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
			cfg := configmock.New(t)
			collector := NewWorkloadMetaCollector(context.Background(), cfg, nil, nil)

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

	podTaggerEntityID := types.NewEntityID(types.KubernetesPodUID, podEntityID.ID)
	containerTaggerEntityID := types.NewEntityID(types.ContainerID, containerID)

	store := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		fx.Provide(func() log.Component { return logmock.New(t) }),
		config.MockModule(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	store.Set(&workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   containerID,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: containerName,
		},
	})

	cfg := configmock.New(t)
	collector := NewWorkloadMetaCollector(context.Background(), cfg, store, nil)

	collector.handleKubePod(workloadmeta.Event{
		Type:   workloadmeta.EventTypeSet,
		Entity: pod,
	})

	expected := []*types.TagInfo{
		{
			Source:       podSource,
			EntityID:     podTaggerEntityID,
			DeleteEntity: true,
		},
		{
			Source:       podSource,
			EntityID:     containerTaggerEntityID,
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
	ch chan []*types.TagInfo
}

func (p *fakeProcessor) ProcessTagInfo(tagInfos []*types.TagInfo) {
	p.ch <- tagInfos
}

func TestHandlePodWithDeletedContainer(t *testing.T) {
	// This test checks that we get events to delete a container that no longer
	// exists even if it belonged to a pod that still exists.

	containerToBeDeletedID := "delete"
	containerToBeDeletedTaggerEntityID := types.NewEntityID(types.ContainerID, containerToBeDeletedID)

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
	podTaggerEntityID := types.NewEntityID(types.KubernetesPodUID, pod.ID)

	collectorCh := make(chan []*types.TagInfo, 10)

	fakeStore := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		fx.Provide(func() log.Component { return logmock.New(t) }),
		config.MockModule(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
	collector := NewWorkloadMetaCollector(context.Background(), configmock.New(t), fakeStore, &fakeProcessor{collectorCh})
	collector.children = map[types.EntityID]map[types.EntityID]struct{}{
		// Notice that here we set the container that belonged to the pod
		// but that no longer exists
		podTaggerEntityID: {containerToBeDeletedTaggerEntityID: struct{}{}}}

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

	expected := &types.TagInfo{
		Source:       podSource,
		EntityID:     containerToBeDeletedTaggerEntityID,
		DeleteEntity: true,
	}

	// We should receive an event to set the pod and another to delete the
	// container. Here we're only interested in the latter, because the former
	// is already checked in other tests.
	found := false
	for evBundle := range collectorCh {
		for _, event := range evBundle {
			if reflect.DeepEqual(event, expected) {
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
			tags := taglist.NewTagList()
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

func assertTagInfoEqual(t *testing.T, expected *types.TagInfo, item *types.TagInfo) bool {
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

func assertTagInfoListEqual(t *testing.T, expectedUpdates []*types.TagInfo, updates []*types.TagInfo) {
	t.Helper()
	require.Equal(t, len(expectedUpdates), len(updates))
	for i := 0; i < len(expectedUpdates); i++ {
		assertTagInfoEqual(t, expectedUpdates[i], updates[i])
	}
}
