// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package suite

import (
	"fmt"
	"strings"
	"sync"

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
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	vpav1 "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1"

	model "github.com/DataDog/agent-payload/v5/process"
)

// resourceFixture is a resource of a built-in kind, with the type of its structured metadata payload.
type resourceFixture struct {
	Object runtime.Object
	// MetadataType is the structured metadata payload type, or nil for a resource that only has a manifest.
	MetadataType model.MessageBody
}

// resourceFixtures returns a resource for each version of each built-in resource of the suite, by collector full name.
func resourceFixtures() map[string]resourceFixture {
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

	return map[string]resourceFixture{
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

// customResourceFixture is a generated object of a custom resource entry, with the API resource that discovery reports
// for it.
type customResourceFixture struct {
	Object      *unstructured.Unstructured
	APIResource metav1.APIResource
}

// customResourceFixtures returns a generated object for each version of each custom resource entry of the suite, by
// collector full name. Custom resources are processed as generic objects, so a generated object covers the entry.
func customResourceFixtures() (map[string]customResourceFixture, error) {
	fixtures := map[string]customResourceFixture{}
	for _, resource := range Resources() {
		if !resource.IsCustomResource() {
			continue
		}
		kind, err := kindOf(resource.fixtureResource())
		if err != nil {
			return nil, err
		}
		for _, version := range resource.Versions {
			fixtures[resource.FixtureName(version)] = customResourceFixture{
				Object: &unstructured.Unstructured{Object: map[string]interface{}{
					"apiVersion": schema.GroupVersion{Group: resource.Group, Version: version}.String(),
					"kind":       kind,
					"metadata": map[string]interface{}{
						"name": "name", "namespace": "default", "uid": "uid", "resourceVersion": "1",
						"annotations": map[string]interface{}{"kubectl.kubernetes.io/last-applied-configuration": "secret"},
					},
					"spec": map[string]interface{}{"replicas": int64(1)},
				}},
				APIResource: metav1.APIResource{Name: resource.fixtureResource(), Kind: kind, Namespaced: true, Verbs: []string{"list", "watch"}},
			}
		}
	}
	return fixtures, nil
}

// kindOf returns the kind of a generated custom resource: the singular of its resource name.
func kindOf(resource string) (string, error) {
	singular := strings.TrimSuffix(resource, "s")
	if base, found := strings.CutSuffix(resource, "ies"); found {
		singular = base + "y"
	} else if base, found := strings.CutSuffix(resource, "sses"); found {
		singular = base + "ss"
	}
	if singular == "" {
		return "", fmt.Errorf("no kind for the resource %q", resource)
	}
	return strings.ToUpper(singular[:1]) + singular[1:], nil
}

// customResourceOfKind maps the kind of each generated custom resource fixture to its resource.
var customResourceOfKind = sync.OnceValues(func() (map[schema.GroupVersionKind]schema.GroupVersionResource, error) {
	fixtures, err := customResourceFixtures()
	if err != nil {
		return nil, err
	}
	resources := make(map[schema.GroupVersionKind]schema.GroupVersionResource, len(fixtures))
	for _, fixture := range fixtures {
		gvk := fixture.Object.GroupVersionKind()
		resources[gvk] = gvk.GroupVersion().WithResource(fixture.APIResource.Name)
	}
	return resources, nil
})

// resourceOf returns the resource of a kind: the resource of a generated custom resource fixture, or the guessed one,
// which is wrong for kinds such as Gateway.
func resourceOf(gvk schema.GroupVersionKind) schema.GroupVersionResource {
	resources, err := customResourceOfKind()
	if err != nil {
		panic(fmt.Sprintf("cannot generate the custom resource fixtures: %v", err))
	}
	if gvr, found := resources[gvk]; found {
		return gvr
	}
	gvr, _ := meta.UnsafeGuessKindToResource(gvk)
	return gvr
}
