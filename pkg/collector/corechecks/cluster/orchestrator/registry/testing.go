// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package registry

import (
	"testing"
	"time"

	"go.uber.org/atomic"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	batchv1beta1 "k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsinformers "k8s.io/apiextensions-apiserver/pkg/client/informers/externalversions"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	vpav1 "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1"
	vpainformers "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/client/informers/externalversions"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/metadata/metadatainformer"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	orchestratorconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
)

// NewTestRunConfig returns a run configuration whose informer factories serve the objects from fake clients, and a
// function that starts the factories. Each object goes to the fake client of its API: metadata-only objects, CRDs,
// VPAs, unstructured objects and typed objects. The factories stop when the test ends.
func NewTestRunConfig(t testing.TB, objects []runtime.Object) (*collectors.CollectorRunConfig, func()) {
	cluster := NewTestCluster(t, objects)
	client, metadataClient, dynamicClient := cluster.client, cluster.metadata, cluster.dynamic
	crdClient, vpaClient := cluster.crds, cluster.vpas

	resync := 300 * time.Second
	factory := &collectors.OrchestratorInformerFactory{
		InformerFactory:               informers.NewSharedInformerFactoryWithOptions(client, resync),
		UnassignedPodInformerFactory:  informers.NewSharedInformerFactoryWithOptions(client, resync),
		DynamicInformerFactory:        dynamicinformer.NewDynamicSharedInformerFactory(dynamicClient, resync),
		CRDInformerFactory:            apiextensionsinformers.NewSharedInformerFactory(crdClient, resync),
		VPAInformerFactory:            vpainformers.NewSharedInformerFactory(vpaClient, resync),
		SecretMetadataInformerFactory: metadatainformer.NewSharedInformerFactory(metadataClient, resync),
		ConfigMapMetadataInformerFactory: metadatainformer.NewFilteredSharedInformerFactory(metadataClient, resync, metav1.NamespaceAll,
			func(opts *metav1.ListOptions) { opts.LabelSelector = NonHelmReleaseLabelSelector }),
		HelmReleaseConfigMapInformerFactory: informers.NewSharedInformerFactoryWithOptions(client, resync,
			informers.WithTweakListOptions(func(opts *metav1.ListOptions) { opts.LabelSelector = HelmReleaseLabelSelector })),
	}

	orchestratorCfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	orchestratorCfg.KubeClusterName = "test-cluster"

	stopCh := make(chan struct{})
	t.Cleanup(func() { close(stopCh) })
	runCfg := &collectors.CollectorRunConfig{
		K8sCollectorRunConfig: collectors.K8sCollectorRunConfig{
			APIClient:                   cluster.APIClient,
			OrchestratorInformerFactory: factory,
		},
		ClusterID:   "test-cluster",
		Config:      orchestratorCfg,
		MsgGroupRef: atomic.NewInt32(0),
		StopCh:      stopCh,
	}

	start := func() {
		factory.InformerFactory.Start(stopCh)
		factory.UnassignedPodInformerFactory.Start(stopCh)
		factory.DynamicInformerFactory.Start(stopCh)
		factory.CRDInformerFactory.Start(stopCh)
		factory.VPAInformerFactory.Start(stopCh)
		factory.SecretMetadataInformerFactory.Start(stopCh)
		factory.ConfigMapMetadataInformerFactory.Start(stopCh)
		factory.HelmReleaseConfigMapInformerFactory.Start(stopCh)
	}
	return runCfg, start
}

// ResourceFixture is a resource of a built-in kind, with the type of its structured metadata payload.
type ResourceFixture struct {
	Object runtime.Object
	// MetadataType is the structured metadata payload type, or nil for a resource that only has a manifest.
	MetadataType model.MessageBody
}

// ResourceFixtures returns a resource for each version of each built-in entry of the registry, by collector full name.
func ResourceFixtures() map[string]ResourceFixture {
	objectMeta := metav1.ObjectMeta{
		Name:            "name",
		Namespace:       "default",
		UID:             "uid",
		ResourceVersion: "1",
		Labels:          map[string]string{"app": "name", "tags.datadoghq.com/env": "prod"},
		Annotations:     map[string]string{"kubectl.kubernetes.io/last-applied-configuration": "secret"},
	}
	helmMeta := *objectMeta.DeepCopy()
	helmMeta.Labels["owner"] = "helm"
	podSpec := corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "image", Env: []corev1.EnvVar{{Name: "PASSWORD", Value: "secret"}}}}}
	podTemplate := corev1.PodTemplateSpec{Spec: podSpec}
	unstructuredMeta := map[string]interface{}{"name": "name", "namespace": "default", "uid": "uid", "resourceVersion": "1"}

	return map[string]ResourceFixture{
		"apiextensions.k8s.io/v1/customresourcedefinitions": {Object: &apiextensionsv1.CustomResourceDefinition{ObjectMeta: objectMeta}},
		"apps/v1/daemonsets":                           {&appsv1.DaemonSet{ObjectMeta: objectMeta, Spec: appsv1.DaemonSetSpec{Template: podTemplate}}, &model.CollectorDaemonSet{}},
		"apps/v1/deployments":                          {&appsv1.Deployment{ObjectMeta: objectMeta, Spec: appsv1.DeploymentSpec{Template: podTemplate}}, &model.CollectorDeployment{}},
		"apps/v1/replicasets":                          {&appsv1.ReplicaSet{ObjectMeta: objectMeta, Spec: appsv1.ReplicaSetSpec{Template: podTemplate}}, &model.CollectorReplicaSet{}},
		"apps/v1/statefulsets":                         {&appsv1.StatefulSet{ObjectMeta: objectMeta, Spec: appsv1.StatefulSetSpec{Template: podTemplate}}, &model.CollectorStatefulSet{}},
		"autoscaling.k8s.io/v1/verticalpodautoscalers": {&vpav1.VerticalPodAutoscaler{ObjectMeta: objectMeta, Spec: vpav1.VerticalPodAutoscalerSpec{TargetRef: &autoscalingv1.CrossVersionObjectReference{Kind: "Deployment", Name: "name"}}}, &model.CollectorVerticalPodAutoscaler{}},
		"autoscaling/v2/horizontalpodautoscalers":      {&autoscalingv2.HorizontalPodAutoscaler{ObjectMeta: objectMeta}, &model.CollectorHorizontalPodAutoscaler{}},
		"batch/v1/cronjobs":                            {&batchv1.CronJob{ObjectMeta: objectMeta, Spec: batchv1.CronJobSpec{JobTemplate: batchv1.JobTemplateSpec{Spec: batchv1.JobSpec{Template: podTemplate}}}}, &model.CollectorCronJob{}},
		"batch/v1beta1/cronjobs":                       {&batchv1beta1.CronJob{ObjectMeta: objectMeta, Spec: batchv1beta1.CronJobSpec{JobTemplate: batchv1beta1.JobTemplateSpec{Spec: batchv1.JobSpec{Template: podTemplate}}}}, &model.CollectorCronJob{}},
		"batch/v1/jobs":                                {&batchv1.Job{ObjectMeta: objectMeta, Spec: batchv1.JobSpec{Template: podTemplate}}, &model.CollectorJob{}},
		"discovery.k8s.io/v1/endpointslices": {Object: &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "discovery.k8s.io/v1",
			"kind":       "EndpointSlice",
			"metadata":   unstructuredMeta,
		}}},
		"networking.k8s.io/v1/ingresses":                   {&networkingv1.Ingress{ObjectMeta: objectMeta}, &model.CollectorIngress{}},
		"networking.k8s.io/v1/networkpolicies":             {&networkingv1.NetworkPolicy{ObjectMeta: objectMeta}, &model.CollectorNetworkPolicy{}},
		"policy/v1/poddisruptionbudgets":                   {&policyv1.PodDisruptionBudget{ObjectMeta: objectMeta}, &model.CollectorPodDisruptionBudget{}},
		"rbac.authorization.k8s.io/v1/clusterrolebindings": {&rbacv1.ClusterRoleBinding{ObjectMeta: objectMeta}, &model.CollectorClusterRoleBinding{}},
		"rbac.authorization.k8s.io/v1/clusterroles":        {&rbacv1.ClusterRole{ObjectMeta: objectMeta}, &model.CollectorClusterRole{}},
		"rbac.authorization.k8s.io/v1/rolebindings":        {&rbacv1.RoleBinding{ObjectMeta: objectMeta}, &model.CollectorRoleBinding{}},
		"rbac.authorization.k8s.io/v1/roles":               {&rbacv1.Role{ObjectMeta: objectMeta}, &model.CollectorRole{}},
		"storage.k8s.io/v1/storageclasses":                 {&storagev1.StorageClass{ObjectMeta: objectMeta}, &model.CollectorStorageClass{}},
		"v1/configmaps":                                    {Object: &metav1.PartialObjectMetadata{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"}, ObjectMeta: objectMeta}},
		"v1/helm-releases-configmaps":                      {Object: &corev1.ConfigMap{ObjectMeta: helmMeta, Data: map[string]string{"release": "data"}}},
		"v1/limitranges":                                   {&corev1.LimitRange{ObjectMeta: objectMeta}, &model.CollectorLimitRange{}},
		"v1/namespaces":                                    {&corev1.Namespace{ObjectMeta: objectMeta}, &model.CollectorNamespace{}},
		"v1/nodes":                                         {&corev1.Node{ObjectMeta: objectMeta}, &model.CollectorNode{}},
		"v1/persistentvolumeclaims":                        {&corev1.PersistentVolumeClaim{ObjectMeta: objectMeta}, &model.CollectorPersistentVolumeClaim{}},
		"v1/persistentvolumes":                             {&corev1.PersistentVolume{ObjectMeta: objectMeta}, &model.CollectorPersistentVolume{}},
		"v1/pods":                                          {&corev1.Pod{ObjectMeta: objectMeta, Spec: podSpec}, &model.CollectorPod{}},
		"v1/secrets":                                       {Object: &metav1.PartialObjectMetadata{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"}, ObjectMeta: objectMeta}},
		"v1/serviceaccounts":                               {&corev1.ServiceAccount{ObjectMeta: objectMeta}, &model.CollectorServiceAccount{}},
		"v1/services":                                      {&corev1.Service{ObjectMeta: objectMeta}, &model.CollectorService{}},
		"v1/terminated-pods":                               {&corev1.Pod{ObjectMeta: objectMeta, Spec: podSpec}, &model.CollectorPod{}},
	}
}
