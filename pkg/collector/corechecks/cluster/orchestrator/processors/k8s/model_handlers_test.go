// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package k8s

import (
	"reflect"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	batchv1beta1 "k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	model "github.com/DataDog/agent-payload/v5/process"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	wmutil "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processorstest"
	"github.com/DataDog/datadog-agent/pkg/orchestrator/config"
)

type entityScope int

const (
	namespacedMetadata entityScope = iota
	clusterMetadata
	deploymentEntity
	nodeEntity
)

// TestModelHandlersWiring checks the ModelSpec of each kind: its tagger entity, YAML field, scrubbed pod template and
// message body. The shared behavior of the handlers is tested in the common package.
func TestModelHandlersWiring(t *testing.T) {
	deploymentTemplate := func(r metav1.Object) *corev1.PodTemplateSpec { return &r.(*appsv1.Deployment).Spec.Template }
	for _, tc := range []struct {
		name        string
		newHandlers func(tagger.Component) processors.Handlers
		resource    metav1.Object
		scope       entityScope
		hasYAML     bool
		podTemplate func(metav1.Object) *corev1.PodTemplateSpec
	}{
		{name: "clusterroles", newHandlers: handlersOf(NewClusterRoleHandlers), resource: createTestClusterRole("name", ""), scope: clusterMetadata, hasYAML: true},
		{name: "clusterrolebindings", newHandlers: handlersOf(NewClusterRoleBindingHandlers), resource: createTestClusterRoleBinding("name", ""), scope: clusterMetadata, hasYAML: true},
		{name: "cronjobs v1", newHandlers: handlersOf(NewCronJobV1Handlers), resource: createTestCronJobV1("name", "default"), hasYAML: true,
			podTemplate: func(r metav1.Object) *corev1.PodTemplateSpec {
				return &r.(*batchv1.CronJob).Spec.JobTemplate.Spec.Template
			}},
		{name: "cronjobs v1beta1", newHandlers: handlersOf(NewCronJobV1Beta1Handlers), resource: createTestCronJobV1Beta1("name", "default"), hasYAML: true,
			podTemplate: func(r metav1.Object) *corev1.PodTemplateSpec {
				return &r.(*batchv1beta1.CronJob).Spec.JobTemplate.Spec.Template
			}},
		{name: "daemonsets", newHandlers: handlersOf(NewDaemonSetHandlers), resource: createTestDaemonSet("name", "default"), hasYAML: true,
			podTemplate: func(r metav1.Object) *corev1.PodTemplateSpec { return &r.(*appsv1.DaemonSet).Spec.Template }},
		{name: "deployments", newHandlers: handlersOf(NewDeploymentHandlers), resource: createTestDeployment("name", "default"), scope: deploymentEntity, hasYAML: true,
			podTemplate: deploymentTemplate},
		{name: "horizontalpodautoscalers", newHandlers: handlersOf(NewHorizontalPodAutoscalerHandlers), resource: createTestHorizontalPodAutoscaler("name", "default"), hasYAML: true},
		{name: "ingresses", newHandlers: handlersOf(NewIngressHandlers), resource: createTestIngress("name", "default"), hasYAML: true},
		{name: "jobs", newHandlers: handlersOf(NewJobHandlers), resource: createTestJob("name", "default"), hasYAML: true,
			podTemplate: func(r metav1.Object) *corev1.PodTemplateSpec { return &r.(*batchv1.Job).Spec.Template }},
		{name: "limitranges", newHandlers: handlersOf(NewLimitRangeHandlers), resource: createTestLimitRange("name", "default")},
		{name: "namespaces", newHandlers: handlersOf(NewNamespaceHandlers), resource: createTestNamespace("name"), scope: clusterMetadata, hasYAML: true},
		{name: "networkpolicies", newHandlers: handlersOf(NewNetworkPolicyHandlers), resource: createTestNetworkPolicy("name", "default"), hasYAML: true},
		{name: "nodes", newHandlers: handlersOf(NewNodeHandlers), resource: createTestNode("name", ""), scope: nodeEntity, hasYAML: true},
		{name: "persistentvolumes", newHandlers: handlersOf(NewPersistentVolumeHandlers), resource: createTestPersistentVolume(), scope: clusterMetadata, hasYAML: true},
		{name: "persistentvolumeclaims", newHandlers: handlersOf(NewPersistentVolumeClaimHandlers), resource: createTestPersistentVolumeClaim(), hasYAML: true},
		{name: "poddisruptionbudgets", newHandlers: handlersOf(NewPodDisruptionBudgetHandlers), resource: createTestPodDisruptionBudget()},
		{name: "replicasets", newHandlers: handlersOf(NewReplicaSetHandlers), resource: createTestReplicaSet(), hasYAML: true,
			podTemplate: func(r metav1.Object) *corev1.PodTemplateSpec { return &r.(*appsv1.ReplicaSet).Spec.Template }},
		{name: "roles", newHandlers: handlersOf(NewRoleHandlers), resource: createTestRole(), hasYAML: true},
		{name: "rolebindings", newHandlers: handlersOf(NewRoleBindingHandlers), resource: createTestRoleBinding(), hasYAML: true},
		{name: "services", newHandlers: handlersOf(NewServiceHandlers), resource: createTestService(), hasYAML: true},
		{name: "serviceaccounts", newHandlers: handlersOf(NewServiceAccountHandlers), resource: createTestServiceAccount(), hasYAML: true},
		{name: "statefulsets", newHandlers: handlersOf(NewStatefulSetHandlers), resource: createTestStatefulSet(), hasYAML: true,
			podTemplate: func(r metav1.Object) *corev1.PodTemplateSpec { return &r.(*appsv1.StatefulSet).Spec.Template }},
		{name: "storageclasses", newHandlers: handlersOf(NewStorageClassHandlers), resource: createTestStorageClass(), scope: clusterMetadata},
		{name: "verticalpodautoscalers", newHandlers: handlersOf(NewVerticalPodAutoscalerHandlers), resource: createTestVerticalPodAutoscaler(), hasYAML: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.NewDefaultOrchestratorConfig([]string{"extra:tag"})
			cfg.KubeClusterName = "cluster"
			cfg.IsScrubbingEnabled = true
			ctx := &processors.K8sProcessorContext{BaseProcessorContext: processors.BaseProcessorContext{
				Cfg: cfg, ClusterID: "cluster-id", MsgGroupID: 3, CollectorTags: []string{"collector:tag"},
				CollectorGroup: "group", CollectorName: tc.name,
			}}
			if tc.podTemplate != nil {
				tc.podTemplate(tc.resource).Spec.Containers = []corev1.Container{{Name: "app", Env: []corev1.EnvVar{{Name: "DB_PASSWORD", Value: "secret"}}}}
			}
			fakeTagger := processorstest.NewFakeTagger(map[taggertypes.EntityID][]string{expectedEntity(tc.scope, ctx, tc.resource): {"tagger:tag"}})
			h := tc.newHandlers(fakeTagger)

			m := h.ExtractResource(ctx, tc.resource)
			extractedTags := slices.Clone(h.GetMetadataTags(ctx, m))
			h.EnrichModel(ctx, tc.resource, m)
			h.ScrubBeforeMarshalling(ctx, tc.resource)
			h.AfterMarshalling(ctx, tc.resource, m, []byte("yaml"))

			type wiring struct {
				Tags     []string
				YAML     []byte
				Scrubbed bool
				Message  model.MessageBody
			}
			want := wiring{Tags: append(extractedTags, "tagger:tag"), Scrubbed: tc.podTemplate != nil, Message: expectedMessage(h, m)}
			if tc.hasYAML {
				want.YAML = []byte("yaml")
			}
			got := wiring{Tags: h.GetMetadataTags(ctx, m), YAML: yamlOf(m), Message: h.BuildMessageBody(ctx, []interface{}{m}, 2)}
			if tc.podTemplate != nil {
				got.Scrubbed = tc.podTemplate(tc.resource).Spec.Containers[0].Env[0].Value != "secret"
			}
			assert.Equal(t, want, got)
		})
	}
}

func handlersOf[H processors.Handlers](newHandlers func(tagger.Component) H) func(tagger.Component) processors.Handlers {
	return func(t tagger.Component) processors.Handlers { return newHandlers(t) }
}

func expectedEntity(scope entityScope, ctx processors.ProcessorContext, r metav1.Object) taggertypes.EntityID {
	switch scope {
	case deploymentEntity:
		return taggertypes.NewEntityID(taggertypes.KubernetesDeployment, r.GetNamespace()+"/"+r.GetName())
	case nodeEntity:
		return taggertypes.NewEntityID(taggertypes.KubernetesNode, r.GetName())
	case clusterMetadata:
		return taggertypes.NewEntityID(taggertypes.KubernetesMetadata,
			string(wmutil.GenerateKubeMetadataEntityID(ctx.GetCollectorGroup(), ctx.GetCollectorName(), "", r.GetName())))
	default:
		return taggertypes.NewEntityID(taggertypes.KubernetesMetadata,
			string(wmutil.GenerateKubeMetadataEntityID(ctx.GetCollectorGroup(), ctx.GetCollectorName(), r.GetNamespace(), r.GetName())))
	}
}

// expectedMessage builds the message body that the handlers must return for one model, from the type of message that
// they return: the shared header fields and a list that holds the model.
func expectedMessage(h processors.Handlers, m interface{}) model.MessageBody {
	cfg := config.NewDefaultOrchestratorConfig(nil)
	sample := h.BuildMessageBody(&processors.K8sProcessorContext{BaseProcessorContext: processors.BaseProcessorContext{Cfg: cfg}}, []interface{}{m}, 0)
	message := reflect.New(reflect.TypeOf(sample).Elem())
	fields := message.Elem()
	fields.FieldByName("ClusterName").SetString("cluster")
	fields.FieldByName("ClusterId").SetString("cluster-id")
	fields.FieldByName("GroupId").SetInt(3)
	fields.FieldByName("GroupSize").SetInt(2)
	fields.FieldByName("Tags").Set(reflect.ValueOf([]string{"extra:tag", "collector:tag"}))
	models := reflect.Append(reflect.MakeSlice(reflect.SliceOf(reflect.TypeOf(m)), 0, 1), reflect.ValueOf(m))
	for i := range fields.NumField() {
		if fields.Field(i).Type() == models.Type() {
			fields.Field(i).Set(models)
		}
	}
	return message.Interface().(model.MessageBody)
}

// yamlOf returns the YAML field of a model, or nil for a model without one.
func yamlOf(m interface{}) []byte {
	field := reflect.ValueOf(m).Elem().FieldByName("Yaml")
	if !field.IsValid() || field.Len() == 0 {
		return nil
	}
	return field.Bytes()
}
