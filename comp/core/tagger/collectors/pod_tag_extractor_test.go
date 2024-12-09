// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collectors

import (
	"context"
	"testing"

	"go.uber.org/fx"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

func TestPodTagExtractor(t *testing.T) {
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

	// standardTags := []string{
	// 	fmt.Sprintf("env:%s", env),
	// 	fmt.Sprintf("service:%s", svc),
	// 	fmt.Sprintf("version:%s", version),
	// }

	podEntityID := workloadmeta.EntityID{
		Kind: workloadmeta.KindKubernetesPod,
		ID:   "foobar",
	}

	// podTaggerEntityID := types.NewEntityID(types.KubernetesPodUID, podEntityID.ID)
	// fullyFleshedContainerTaggerEntityID := types.NewEntityID(types.ContainerID, fullyFleshedContainerID)
	// noEnvContainerTaggerEntityID := types.NewEntityID(types.ContainerID, noEnvContainerID)

	// image := workloadmeta.ContainerImage{
	// 	ID:        "datadog/agent@sha256:a63d3f66fb2f69d955d4f2ca0b229385537a77872ffc04290acae65aed5317d2",
	// 	RawName:   "datadog/agent@sha256:a63d3f66fb2f69d955d4f2ca0b229385537a77872ffc04290acae65aed5317d2",
	// 	Name:      "datadog/agent",
	// 	ShortName: "agent",
	// 	Tag:       "latest",
	// }

	basePod := &workloadmeta.KubernetesPod{
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
	}

	tests := []struct {
		name        string
		cardinality types.TagCardinality
		wmetaPod    *workloadmeta.KubernetesPod
		expected    []string
	}{
		{
			name:        "low cardinality",
			cardinality: types.LowCardinality,
			wmetaPod:    basePod,
			expected:    []string{"kube_deployment:datadog-agent", "kube_priority_class:high-priority", "kube_qos:guaranteed", "kube_app_component:agent", "kube_service:service1", "kube_ownerref_kind:deployment", "pod_phase:running", "env:production", "version:7.32.0", "kube_app_name:datadog-agent", "pod_template_version:1.0.0", "kube_namespace:default", "kube_app_instance:datadog-agent-foobar", "kube_app_managed_by:helm", "kube_service:service2", "kube_runtime_class:myclass", "service:datadog-agent", "kube_app_version:7.32.0", "kube_app_part_of:datadog"},
		},
		{
			name:        "orch cardinality",
			cardinality: types.OrchestratorCardinality,
			wmetaPod:    basePod,
			expected: []string{"kube_service:service2", "kube_deployment:datadog-agent", "kube_priority_class:high-priority",
				"kube_qos:guaranteed", "env:production", "kube_app_name:datadog-agent", "kube_app_managed_by:helm", "pod_phase:running",
				"kube_runtime_class:myclass", "kube_service:service1", "pod_template_version:1.0.0", "version:7.32.0", "kube_app_version:7.32.0",
				"kube_app_instance:datadog-agent-foobar", "kube_app_component:agent", "kube_app_part_of:datadog", "kube_ownerref_kind:deployment",
				"kube_namespace:default", "service:datadog-agent", "pod_name:datadog-agent-foobar", "kube_ownerref_name:datadog-agent"},
		},
		{
			name:        "high cardinality",
			cardinality: types.HighCardinality,
			wmetaPod:    basePod,
			expected: []string{"kube_qos:guaranteed", "kube_app_part_of:datadog", "kube_app_version:7.32.0", "pod_template_version:1.0.0",
				"kube_deployment:datadog-agent", "kube_priority_class:high-priority", "kube_app_managed_by:helm", "version:7.32.0",
				"kube_app_instance:datadog-agent-foobar", "kube_service:service2", "kube_app_name:datadog-agent", "kube_app_component:agent",
				"service:datadog-agent", "kube_namespace:default", "pod_phase:running", "kube_runtime_class:myclass", "env:production",
				"kube_service:service1", "kube_ownerref_kind:deployment", "pod_name:datadog-agent-foobar", "kube_ownerref_name:datadog-agent"},
		},
		{
			name:        "none cardinality",
			cardinality: types.NoneCardinality,
			wmetaPod:    basePod,
			expected:    []string{},
		},
		{
			name:        "unsupported cardinality",
			cardinality: types.TagCardinality(100),
			wmetaPod:    basePod,
			expected:    []string{},
		},
		{
			name:        "nil pod",
			cardinality: types.LowCardinality,
			wmetaPod:    nil,
			expected:    []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig := configmock.New(t)
			mockStore := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
				fx.Provide(func() log.Component { return logmock.New(t) }),
				config.MockModule(),
				fx.Supply(context.Background()),
				workloadmetafxmock.MockModule(workloadmeta.NewParams()),
			))

			p := NewPodTagExtractor(mockConfig, mockStore)
			tags := p.Extract(tt.wmetaPod, tt.cardinality)

			assert.NotNil(t, tags)
			assert.ElementsMatch(t, tt.expected, tags)
		})
	}
}
