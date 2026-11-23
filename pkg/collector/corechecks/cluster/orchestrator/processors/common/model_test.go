// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	model "github.com/DataDog/agent-payload/v5/process"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processorstest"
	"github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/redact"
)

func TestModelHandlers(t *testing.T) {
	entity := taggertypes.NewEntityID(taggertypes.KubernetesDeployment, "default/name")
	fakeTagger := processorstest.NewFakeTagger(map[taggertypes.EntityID][]string{entity: {"team:a"}})

	scrubbed := false
	h := NewModelHandlers(fakeTagger, ModelSpec[*appsv1.Deployment, *model.Deployment]{
		Extract: func(_ processors.ProcessorContext, r *appsv1.Deployment) *model.Deployment {
			return &model.Deployment{Metadata: &model.Metadata{Name: r.Name}, Tags: []string{"model:tag"}}
		},
		Tags:   func(m *model.Deployment) *[]string { return &m.Tags },
		YAML:   func(m *model.Deployment) *[]byte { return &m.Yaml },
		Entity: NamespacedEntity[*appsv1.Deployment](taggertypes.KubernetesDeployment),
		Scrub:  func(*appsv1.Deployment, *redact.DataScrubber) { scrubbed = true },
		Message: func(header MessageHeader, models []*model.Deployment) model.MessageBody {
			return &model.CollectorDeployment{ClusterName: header.ClusterName, ClusterId: header.ClusterID, GroupId: header.GroupID,
				GroupSize: header.GroupSize, Deployments: models, Tags: header.Tags, AgentVersion: header.AgentVersion}
		},
	})

	cfg := config.NewDefaultOrchestratorConfig([]string{"extra:tag"})
	cfg.KubeClusterName = "cluster"
	ctx := &processors.K8sProcessorContext{BaseProcessorContext: processors.BaseProcessorContext{
		Cfg: cfg, ClusterID: "cluster-id", MsgGroupID: 3, CollectorTags: []string{"kube_api_version:apps/v1"},
	}}
	deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "name", Namespace: "default"}}

	m := h.ExtractResource(ctx, deployment).(*model.Deployment)
	assert.False(t, h.EnrichModel(ctx, deployment, m))
	assert.Equal(t, []string{"model:tag", "team:a"}, m.Tags)
	assert.Equal(t, []string{"model:tag", "team:a"}, h.GetMetadataTags(ctx, m))
	assert.Nil(t, h.GetMetadataTags(ctx, &model.Pod{}))

	assert.False(t, h.AfterMarshalling(ctx, deployment, m, []byte("yaml")))
	assert.Equal(t, []byte("yaml"), m.Yaml)

	cfg.IsScrubbingEnabled = false
	h.ScrubBeforeMarshalling(ctx, deployment)
	assert.False(t, scrubbed)
	cfg.IsScrubbingEnabled = true
	h.ScrubBeforeMarshalling(ctx, deployment)
	assert.True(t, scrubbed)

	assert.Equal(t, &model.CollectorDeployment{
		ClusterName: "cluster",
		ClusterId:   "cluster-id",
		GroupId:     3,
		GroupSize:   2,
		Deployments: []*model.Deployment{m},
		Tags:        []string{"extra:tag", "kube_api_version:apps/v1"},
	}, h.BuildMessageBody(ctx, []interface{}{m}, 2))
}

// TestModelHandlersOptionalFields checks the handlers of a resource without tagger tags, YAML or scrubbing.
func TestModelHandlersOptionalFields(t *testing.T) {
	h := NewModelHandlers(nil, ModelSpec[*corev1.LimitRange, *model.LimitRange]{
		Extract: func(processors.ProcessorContext, *corev1.LimitRange) *model.LimitRange { return &model.LimitRange{} },
		Tags:    func(m *model.LimitRange) *[]string { return &m.Tags },
	})
	// The context is not a Kubernetes processor context: handlers without options must not read it.
	ctx := &processors.BaseProcessorContext{}
	m := &model.LimitRange{}

	assert.False(t, h.EnrichModel(ctx, &corev1.LimitRange{}, m))
	assert.False(t, h.AfterMarshalling(ctx, nil, m, []byte("yaml")))
	h.ScrubBeforeMarshalling(ctx, &corev1.LimitRange{})
	assert.Empty(t, m.Tags)
}

func TestTaggerEntities(t *testing.T) {
	ctx := &processors.BaseProcessorContext{CollectorGroup: "rbac.authorization.k8s.io", CollectorName: "roles"}
	role := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "name", Namespace: "default"}}

	assert.Equal(t, taggertypes.NewEntityID(taggertypes.KubernetesDeployment, "default/name"), NamespacedEntity[*corev1.ConfigMap](taggertypes.KubernetesDeployment)(ctx, role))
	assert.Equal(t, taggertypes.KubernetesMetadata, MetadataEntity(ctx, role).GetPrefix())
	assert.Contains(t, MetadataEntity(ctx, role).GetID(), "default")
	assert.NotContains(t, ClusterMetadataEntity(ctx, role).GetID(), "default")
}
