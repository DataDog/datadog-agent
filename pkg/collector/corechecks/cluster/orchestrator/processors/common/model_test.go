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
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	model "github.com/DataDog/agent-payload/v5/process"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	wmutil "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processorstest"
	"github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/redact"
)

func deploymentSpec(scrubbed *[]string) ModelSpec[*appsv1.Deployment, *model.Deployment] {
	return ModelSpec[*appsv1.Deployment, *model.Deployment]{
		Extract: func(_ processors.ProcessorContext, r *appsv1.Deployment) *model.Deployment {
			return &model.Deployment{Metadata: &model.Metadata{Name: r.Name}, Tags: []string{"model:tag"}}
		},
		Tags: func(m *model.Deployment) *[]string { return &m.Tags },
		YAML: func(m *model.Deployment) *[]byte { return &m.Yaml },
		Entity: func(_ processors.ProcessorContext, r *appsv1.Deployment) taggertypes.EntityID {
			return taggertypes.NewEntityID(taggertypes.KubernetesDeployment, r.Namespace+"/"+r.Name)
		},
		Scrub: func(r *appsv1.Deployment, _ *redact.DataScrubber) { *scrubbed = append(*scrubbed, r.Name) },
		Message: func(header MessageHeader, models []*model.Deployment) model.MessageBody {
			return &model.CollectorDeployment{ClusterName: header.ClusterName, ClusterId: header.ClusterID, GroupId: header.GroupID,
				GroupSize: header.GroupSize, Deployments: models, Tags: header.Tags, AgentVersion: header.AgentVersion}
		},
	}
}

// TestModelHandlers runs a resource through the handlers in the order that the processor calls them.
func TestModelHandlers(t *testing.T) {
	entity := taggertypes.NewEntityID(taggertypes.KubernetesDeployment, "default/name")
	fakeTagger := processorstest.NewFakeTagger(map[taggertypes.EntityID][]string{entity: {"team:a"}})
	var scrubbed []string
	h := NewModelHandlers(fakeTagger, deploymentSpec(&scrubbed))

	cfg := config.NewDefaultOrchestratorConfig([]string{"extra:tag"})
	cfg.KubeClusterName = "cluster"
	ctx := &processors.K8sProcessorContext{BaseProcessorContext: processors.BaseProcessorContext{
		Cfg: cfg, ClusterID: "cluster-id", MsgGroupID: 3, CollectorTags: []string{"kube_api_version:apps/v1"},
	}}
	deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "name", Namespace: "default"}}

	m := h.ExtractResource(ctx, deployment).(*model.Deployment)
	assert.False(t, h.EnrichModel(ctx, deployment, m))
	cfg.IsScrubbingEnabled = false
	h.ScrubBeforeMarshalling(ctx, deployment)
	cfg.IsScrubbingEnabled = true
	h.ScrubBeforeMarshalling(ctx, deployment)
	assert.False(t, h.AfterMarshalling(ctx, deployment, m, []byte("yaml")))

	expectedModel := &model.Deployment{Metadata: &model.Metadata{Name: "name"}, Tags: []string{"model:tag", "team:a"}, Yaml: []byte("yaml")}
	assert.Equal(t, expectedModel, m)
	assert.Equal(t, expectedModel.Tags, h.GetMetadataTags(ctx, m))
	assert.Nil(t, h.GetMetadataTags(ctx, &model.Pod{}), "a model of another type has no tags")
	assert.Equal(t, []string{"name"}, scrubbed, "only scrubs when scrubbing is enabled")
	assert.Equal(t, &model.CollectorDeployment{
		ClusterName: "cluster",
		ClusterId:   "cluster-id",
		GroupId:     3,
		GroupSize:   2,
		Deployments: []*model.Deployment{expectedModel},
		Tags:        []string{"extra:tag", "kube_api_version:apps/v1"},
	}, h.BuildMessageBody(ctx, []interface{}{m}, 2))
}

// TestModelHandlersOptionalFields checks the handlers of a resource without tagger tags, YAML or scrubbing.
func TestModelHandlersOptionalFields(t *testing.T) {
	h := NewModelHandlers(nil, ModelSpec[*corev1.LimitRange, *model.LimitRange]{
		Extract: func(processors.ProcessorContext, *corev1.LimitRange) *model.LimitRange { return &model.LimitRange{} },
		Tags:    func(m *model.LimitRange) *[]string { return &m.Tags },
		Message: func(MessageHeader, []*model.LimitRange) model.MessageBody { return &model.CollectorLimitRange{} },
	})
	// The context is not a Kubernetes processor context: handlers without options must not read it.
	ctx := &processors.BaseProcessorContext{}
	m := &model.LimitRange{}

	assert.False(t, h.EnrichModel(ctx, &corev1.LimitRange{}, m))
	assert.False(t, h.AfterMarshalling(ctx, nil, m, []byte("yaml")))
	h.ScrubBeforeMarshalling(ctx, &corev1.LimitRange{})
	assert.Equal(t, &model.LimitRange{}, m)
}

// TestNewModelHandlersRequiredFields checks that a spec without a required function fails when the handlers are built.
func TestNewModelHandlersRequiredFields(t *testing.T) {
	var scrubbed []string
	for name, drop := range map[string]func(*ModelSpec[*appsv1.Deployment, *model.Deployment]){
		"Extract": func(s *ModelSpec[*appsv1.Deployment, *model.Deployment]) { s.Extract = nil },
		"Tags":    func(s *ModelSpec[*appsv1.Deployment, *model.Deployment]) { s.Tags = nil },
		"Message": func(s *ModelSpec[*appsv1.Deployment, *model.Deployment]) { s.Message = nil },
	} {
		t.Run(name, func(t *testing.T) {
			spec := deploymentSpec(&scrubbed)
			drop(&spec)
			assert.PanicsWithValue(t, "the model spec of *v1.Deployment misses Extract, Tags or Message", func() { NewModelHandlers(nil, spec) })
		})
	}
}

func TestTaggerEntities(t *testing.T) {
	ctx := &processors.BaseProcessorContext{CollectorGroup: "rbac.authorization.k8s.io", CollectorName: "roles"}
	role := &rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Name: "name", Namespace: "default"}}
	type entities struct{ Metadata, ClusterMetadata taggertypes.EntityID }

	assert.Equal(t, entities{
		Metadata:        taggertypes.NewEntityID(taggertypes.KubernetesMetadata, string(wmutil.GenerateKubeMetadataEntityID("rbac.authorization.k8s.io", "roles", "default", "name"))),
		ClusterMetadata: taggertypes.NewEntityID(taggertypes.KubernetesMetadata, string(wmutil.GenerateKubeMetadataEntityID("rbac.authorization.k8s.io", "roles", "", "name"))),
	}, entities{
		Metadata:        MetadataEntity(ctx, role),
		ClusterMetadata: ClusterMetadataEntity(ctx, role),
	})
}
