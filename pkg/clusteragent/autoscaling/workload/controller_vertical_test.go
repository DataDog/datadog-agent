// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package workload

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	v2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/record"
	clock "k8s.io/utils/clock/testing"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"
)

type verticalControllerFixture struct {
	t             *testing.T
	clock         *clock.FakeClock
	recorder      *record.FakeRecorder
	dynamicClient *dynamicfake.FakeDynamicClient
	podWatcher    *fakePodWatcher
	controller    *verticalController
}

func newVerticalControllerFixture(t *testing.T, testTime time.Time) *verticalControllerFixture {
	fakeClock := clock.NewFakeClock(testTime)
	recorder := record.NewFakeRecorder(100)
	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme)
	podWatcher := &fakePodWatcher{}

	return &verticalControllerFixture{
		t:             t,
		clock:         fakeClock,
		recorder:      recorder,
		dynamicClient: dynamicClient,
		podWatcher:    podWatcher,
		controller: &verticalController{
			clock:         fakeClock,
			eventRecorder: recorder,
			dynamicClient: dynamicClient,
		},
	}
}

func TestSyncDeploymentKind_AllPodsUpdated(t *testing.T) {
	testTime := time.Now()
	f := newVerticalControllerFixture(t, testTime)

	autoscalerNamespace := "default"
	autoscalerName := "test-deployment-autoscaler"
	deploymentName := "test-deployment"
	replicaSetName := "test-deployment-abc123"
	recommendationID := "rec-123"

	expectedGVK := schema.GroupVersionKind{
		Group:   "apps",
		Version: "v1",
		Kind:    kubernetes.DeploymentKind,
	}

	fakePai := &model.FakePodAutoscalerInternal{
		Namespace: autoscalerNamespace,
		Name:      autoscalerName,
		Spec: &datadoghq.DatadogPodAutoscalerSpec{
			TargetRef: v2.CrossVersionObjectReference{
				Name:       deploymentName,
				Kind:       expectedGVK.Kind,
				APIVersion: expectedGVK.Group + "/" + expectedGVK.Version,
			},
		},
		TargetGVK:       expectedGVK,
		CurrentReplicas: pointer.Ptr[int32](3),
	}

	// All pods have the recommendation ID - rollout complete
	pods := []*workloadmeta.KubernetesPod{
		{
			EntityMeta: workloadmeta.EntityMeta{
				Name:      "pod-1",
				Namespace: autoscalerNamespace,
				Annotations: map[string]string{
					model.RecommendationIDAnnotation: recommendationID,
				},
			},
			Owners: []workloadmeta.KubernetesPodOwner{
				{Kind: kubernetes.ReplicaSetKind, Name: replicaSetName},
			},
		},
		{
			EntityMeta: workloadmeta.EntityMeta{
				Name:      "pod-2",
				Namespace: autoscalerNamespace,
				Annotations: map[string]string{
					model.RecommendationIDAnnotation: recommendationID,
				},
			},
			Owners: []workloadmeta.KubernetesPodOwner{
				{Kind: kubernetes.ReplicaSetKind, Name: replicaSetName},
			},
		},
		{
			EntityMeta: workloadmeta.EntityMeta{
				Name:      "pod-3",
				Namespace: autoscalerNamespace,
				Annotations: map[string]string{
					model.RecommendationIDAnnotation: recommendationID,
				},
			},
			Owners: []workloadmeta.KubernetesPodOwner{
				{Kind: kubernetes.ReplicaSetKind, Name: replicaSetName},
			},
		},
	}

	podsPerRecomendationID := map[string]int32{
		recommendationID: 3,
	}

	// All pods owned by the same ReplicaSet
	podsPerDirectOwner := map[string]int32{
		replicaSetName: 3,
	}

	target := NamespacedPodOwner{
		Namespace: autoscalerNamespace,
		Kind:      kubernetes.DeploymentKind,
		Name:      deploymentName,
	}

	autoscalerInternal := fakePai.Build()
	fakeAutoscaler := &datadoghq.DatadogPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      autoscalerName,
			Namespace: autoscalerNamespace,
		},
	}

	result, err := f.controller.syncDeploymentKind(
		context.Background(),
		fakeAutoscaler,
		&autoscalerInternal,
		datadoghqcommon.DatadogPodAutoscalerAutoUpdateStrategy,
		target,
		expectedGVK,
		recommendationID,
		pods,
		podsPerRecomendationID,
		podsPerDirectOwner,
	)

	assert.NoError(t, err)
	assert.Equal(t, autoscaling.NoRequeue, result)
	// Vertical action should be cleared (nil, nil) on success
	assert.Nil(t, autoscalerInternal.VerticalLastAction())
	assert.Nil(t, autoscalerInternal.VerticalLastActionError())
}

func TestSyncDeploymentKind_RolloutInProgress(t *testing.T) {
	testTime := time.Now()
	f := newVerticalControllerFixture(t, testTime)

	autoscalerNamespace := "default"
	autoscalerName := "test-deployment-autoscaler"
	deploymentName := "test-deployment"
	oldReplicaSetName := "test-deployment-old123"
	newReplicaSetName := "test-deployment-new456"
	recommendationID := "rec-123"

	expectedGVK := schema.GroupVersionKind{
		Group:   "apps",
		Version: "v1",
		Kind:    kubernetes.DeploymentKind,
	}

	fakePai := &model.FakePodAutoscalerInternal{
		Namespace: autoscalerNamespace,
		Name:      autoscalerName,
		Spec: &datadoghq.DatadogPodAutoscalerSpec{
			TargetRef: v2.CrossVersionObjectReference{
				Name:       deploymentName,
				Kind:       expectedGVK.Kind,
				APIVersion: expectedGVK.Group + "/" + expectedGVK.Version,
			},
		},
		TargetGVK:       expectedGVK,
		CurrentReplicas: pointer.Ptr[int32](3),
	}

	// Pods from two different ReplicaSets - rollout in progress
	pods := []*workloadmeta.KubernetesPod{
		{
			EntityMeta: workloadmeta.EntityMeta{
				Name:      "pod-old-1",
				Namespace: autoscalerNamespace,
				Annotations: map[string]string{
					model.RecommendationIDAnnotation: "old-rec",
				},
			},
			Owners: []workloadmeta.KubernetesPodOwner{
				{Kind: kubernetes.ReplicaSetKind, Name: oldReplicaSetName},
			},
		},
		{
			EntityMeta: workloadmeta.EntityMeta{
				Name:      "pod-new-1",
				Namespace: autoscalerNamespace,
				Annotations: map[string]string{
					model.RecommendationIDAnnotation: recommendationID,
				},
			},
			Owners: []workloadmeta.KubernetesPodOwner{
				{Kind: kubernetes.ReplicaSetKind, Name: newReplicaSetName},
			},
		},
		{
			EntityMeta: workloadmeta.EntityMeta{
				Name:      "pod-new-2",
				Namespace: autoscalerNamespace,
				Annotations: map[string]string{
					model.RecommendationIDAnnotation: recommendationID,
				},
			},
			Owners: []workloadmeta.KubernetesPodOwner{
				{Kind: kubernetes.ReplicaSetKind, Name: newReplicaSetName},
			},
		},
	}

	podsPerRecomendationID := map[string]int32{
		"old-rec":        1,
		recommendationID: 2,
	}

	// Pods from two different ReplicaSets - indicates rollout in progress
	podsPerDirectOwner := map[string]int32{
		oldReplicaSetName: 1,
		newReplicaSetName: 2,
	}

	target := NamespacedPodOwner{
		Namespace: autoscalerNamespace,
		Kind:      kubernetes.DeploymentKind,
		Name:      deploymentName,
	}

	autoscalerInternal := fakePai.Build()
	fakeAutoscaler := &datadoghq.DatadogPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      autoscalerName,
			Namespace: autoscalerNamespace,
		},
	}

	result, err := f.controller.syncDeploymentKind(
		context.Background(),
		fakeAutoscaler,
		&autoscalerInternal,
		datadoghqcommon.DatadogPodAutoscalerAutoUpdateStrategy,
		target,
		expectedGVK,
		recommendationID,
		pods,
		podsPerRecomendationID,
		podsPerDirectOwner,
	)

	assert.NoError(t, err)
	assert.Equal(t, autoscaling.ProcessResult{Requeue: true, RequeueAfter: rolloutCheckRequeueDelay}, result)
}

func TestSyncDeploymentKind_TriggerRollout(t *testing.T) {
	testTime := time.Now()
	f := newVerticalControllerFixture(t, testTime)

	autoscalerNamespace := "default"
	autoscalerName := "test-deployment-autoscaler"
	deploymentName := "test-deployment"
	replicaSetName := "test-deployment-abc123"
	recommendationID := "rec-123"

	expectedGVK := schema.GroupVersionKind{
		Group:   "apps",
		Version: "v1",
		Kind:    kubernetes.DeploymentKind,
	}

	// Create a fake Deployment in the dynamic client
	deployment := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      deploymentName,
				"namespace": autoscalerNamespace,
			},
			"spec": map[string]interface{}{
				"replicas": int64(3),
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"annotations": map[string]interface{}{},
					},
				},
			},
		},
	}

	gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	_, err := f.dynamicClient.Resource(gvr).Namespace(autoscalerNamespace).Create(context.Background(), deployment, metav1.CreateOptions{})
	assert.NoError(t, err)

	fakePai := &model.FakePodAutoscalerInternal{
		Namespace: autoscalerNamespace,
		Name:      autoscalerName,
		Spec: &datadoghq.DatadogPodAutoscalerSpec{
			TargetRef: v2.CrossVersionObjectReference{
				Name:       deploymentName,
				Kind:       expectedGVK.Kind,
				APIVersion: expectedGVK.Group + "/" + expectedGVK.Version,
			},
		},
		TargetGVK:       expectedGVK,
		CurrentReplicas: pointer.Ptr[int32](3),
	}

	// No pods have the new recommendation ID - need to trigger rollout
	pods := []*workloadmeta.KubernetesPod{
		{
			EntityMeta: workloadmeta.EntityMeta{
				Name:      "pod-1",
				Namespace: autoscalerNamespace,
				Annotations: map[string]string{
					model.RecommendationIDAnnotation: "old-rec",
				},
			},
			Owners: []workloadmeta.KubernetesPodOwner{
				{Kind: kubernetes.ReplicaSetKind, Name: replicaSetName},
			},
		},
		{
			EntityMeta: workloadmeta.EntityMeta{
				Name:      "pod-2",
				Namespace: autoscalerNamespace,
				Annotations: map[string]string{
					model.RecommendationIDAnnotation: "old-rec",
				},
			},
			Owners: []workloadmeta.KubernetesPodOwner{
				{Kind: kubernetes.ReplicaSetKind, Name: replicaSetName},
			},
		},
		{
			EntityMeta: workloadmeta.EntityMeta{
				Name:      "pod-3",
				Namespace: autoscalerNamespace,
				Annotations: map[string]string{
					model.RecommendationIDAnnotation: "old-rec",
				},
			},
			Owners: []workloadmeta.KubernetesPodOwner{
				{Kind: kubernetes.ReplicaSetKind, Name: replicaSetName},
			},
		},
	}

	podsPerRecomendationID := map[string]int32{
		"old-rec":        3,
		recommendationID: 0,
	}

	// All pods from the same ReplicaSet - no rollout in progress
	podsPerDirectOwner := map[string]int32{
		replicaSetName: 3,
	}

	target := NamespacedPodOwner{
		Namespace: autoscalerNamespace,
		Kind:      kubernetes.DeploymentKind,
		Name:      deploymentName,
	}

	autoscalerInternal := fakePai.Build()
	fakeAutoscaler := &datadoghq.DatadogPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      autoscalerName,
			Namespace: autoscalerNamespace,
		},
	}

	result, err := f.controller.syncDeploymentKind(
		context.Background(),
		fakeAutoscaler,
		&autoscalerInternal,
		datadoghqcommon.DatadogPodAutoscalerAutoUpdateStrategy,
		target,
		expectedGVK,
		recommendationID,
		pods,
		podsPerRecomendationID,
		podsPerDirectOwner,
	)

	assert.NoError(t, err)
	assert.Equal(t, autoscaling.ProcessResult{Requeue: true, RequeueAfter: rolloutCheckRequeueDelay}, result)

	// Verify vertical action was recorded
	assert.NotNil(t, autoscalerInternal.VerticalLastAction())
	assert.Equal(t, recommendationID, autoscalerInternal.VerticalLastAction().Version)
	assert.Equal(t, datadoghqcommon.DatadogPodAutoscalerRolloutTriggeredVerticalActionType, autoscalerInternal.VerticalLastAction().Type)

	// Verify the Deployment was patched
	updatedDeployment, err := f.dynamicClient.Resource(gvr).Namespace(autoscalerNamespace).Get(context.Background(), deploymentName, metav1.GetOptions{})
	assert.NoError(t, err)

	annotations, found, err := unstructured.NestedStringMap(updatedDeployment.Object, "spec", "template", "metadata", "annotations")
	assert.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, recommendationID, annotations[model.RecommendationIDAnnotation])
	assert.NotEmpty(t, annotations[model.RolloutTimestampAnnotation])
}

func TestSyncDeploymentKind_PatchError(t *testing.T) {
	testTime := time.Now()
	f := newVerticalControllerFixture(t, testTime)

	autoscalerNamespace := "default"
	autoscalerName := "test-deployment-autoscaler"
	deploymentName := "nonexistent-deployment"
	replicaSetName := "nonexistent-deployment-abc123"
	recommendationID := "rec-123"

	expectedGVK := schema.GroupVersionKind{
		Group:   "apps",
		Version: "v1",
		Kind:    kubernetes.DeploymentKind,
	}

	// Don't create the Deployment in the dynamic client - patch will fail

	fakePai := &model.FakePodAutoscalerInternal{
		Namespace: autoscalerNamespace,
		Name:      autoscalerName,
		Spec: &datadoghq.DatadogPodAutoscalerSpec{
			TargetRef: v2.CrossVersionObjectReference{
				Name:       deploymentName,
				Kind:       expectedGVK.Kind,
				APIVersion: expectedGVK.Group + "/" + expectedGVK.Version,
			},
		},
		TargetGVK:       expectedGVK,
		CurrentReplicas: pointer.Ptr[int32](3),
	}

	pods := []*workloadmeta.KubernetesPod{
		{
			EntityMeta: workloadmeta.EntityMeta{
				Name:      "pod-1",
				Namespace: autoscalerNamespace,
			},
			Owners: []workloadmeta.KubernetesPodOwner{
				{Kind: kubernetes.ReplicaSetKind, Name: replicaSetName},
			},
		},
	}

	podsPerRecomendationID := map[string]int32{
		recommendationID: 0,
	}

	podsPerDirectOwner := map[string]int32{
		replicaSetName: 1,
	}

	target := NamespacedPodOwner{
		Namespace: autoscalerNamespace,
		Kind:      kubernetes.DeploymentKind,
		Name:      deploymentName,
	}

	autoscalerInternal := fakePai.Build()
	fakeAutoscaler := &datadoghq.DatadogPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      autoscalerName,
			Namespace: autoscalerNamespace,
		},
	}

	// Add a reactor that returns an error for patch operations
	f.dynamicClient.PrependReactor("patch", "deployments", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, assert.AnError
	})

	result, err := f.controller.syncDeploymentKind(
		context.Background(),
		fakeAutoscaler,
		&autoscalerInternal,
		datadoghqcommon.DatadogPodAutoscalerAutoUpdateStrategy,
		target,
		expectedGVK,
		recommendationID,
		pods,
		podsPerRecomendationID,
		podsPerDirectOwner,
	)

	assert.Error(t, err)
	assert.Equal(t, autoscaling.Requeue, result)
	assert.NotNil(t, autoscalerInternal.VerticalLastActionError())
}

func TestSyncStatefulSetKind_AllPodsUpdated(t *testing.T) {
	testTime := time.Now()
	f := newVerticalControllerFixture(t, testTime)

	autoscalerNamespace := "default"
	autoscalerName := "test-statefulset-autoscaler"
	statefulSetName := "test-statefulset"
	recommendationID := "rec-123"

	expectedGVK := schema.GroupVersionKind{
		Group:   "apps",
		Version: "v1",
		Kind:    kubernetes.StatefulSetKind,
	}

	fakePai := &model.FakePodAutoscalerInternal{
		Namespace: autoscalerNamespace,
		Name:      autoscalerName,
		Spec: &datadoghq.DatadogPodAutoscalerSpec{
			TargetRef: v2.CrossVersionObjectReference{
				Name:       statefulSetName,
				Kind:       expectedGVK.Kind,
				APIVersion: expectedGVK.Group + "/" + expectedGVK.Version,
			},
		},
		TargetGVK:       expectedGVK,
		CurrentReplicas: pointer.Ptr[int32](3),
	}

	// All pods have the recommendation ID - rollout complete
	pods := []*workloadmeta.KubernetesPod{
		{
			EntityMeta: workloadmeta.EntityMeta{
				Name:      statefulSetName + "-0",
				Namespace: autoscalerNamespace,
				Annotations: map[string]string{
					model.RecommendationIDAnnotation: recommendationID,
				},
			},
			Owners: []workloadmeta.KubernetesPodOwner{
				{Kind: kubernetes.StatefulSetKind, Name: statefulSetName},
			},
		},
		{
			EntityMeta: workloadmeta.EntityMeta{
				Name:      statefulSetName + "-1",
				Namespace: autoscalerNamespace,
				Annotations: map[string]string{
					model.RecommendationIDAnnotation: recommendationID,
				},
			},
			Owners: []workloadmeta.KubernetesPodOwner{
				{Kind: kubernetes.StatefulSetKind, Name: statefulSetName},
			},
		},
		{
			EntityMeta: workloadmeta.EntityMeta{
				Name:      statefulSetName + "-2",
				Namespace: autoscalerNamespace,
				Annotations: map[string]string{
					model.RecommendationIDAnnotation: recommendationID,
				},
			},
			Owners: []workloadmeta.KubernetesPodOwner{
				{Kind: kubernetes.StatefulSetKind, Name: statefulSetName},
			},
		},
	}

	podsPerRecomendationID := map[string]int32{
		recommendationID: 3,
	}

	target := NamespacedPodOwner{
		Namespace: autoscalerNamespace,
		Kind:      kubernetes.StatefulSetKind,
		Name:      statefulSetName,
	}

	autoscalerInternal := fakePai.Build()
	fakeAutoscaler := &datadoghq.DatadogPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      autoscalerName,
			Namespace: autoscalerNamespace,
		},
	}

	result, err := f.controller.syncStatefulSetKind(
		context.Background(),
		fakeAutoscaler,
		&autoscalerInternal,
		datadoghqcommon.DatadogPodAutoscalerAutoUpdateStrategy,
		target,
		expectedGVK,
		recommendationID,
		pods,
		podsPerRecomendationID,
	)

	assert.NoError(t, err)
	assert.Equal(t, autoscaling.NoRequeue, result)
	// Vertical action should be cleared (nil, nil) on success
	assert.Nil(t, autoscalerInternal.VerticalLastAction())
	assert.Nil(t, autoscalerInternal.VerticalLastActionError())
}

func TestSyncStatefulSetKind_RolloutInProgress(t *testing.T) {
	testTime := time.Now()
	f := newVerticalControllerFixture(t, testTime)

	autoscalerNamespace := "default"
	autoscalerName := "test-statefulset-autoscaler"
	statefulSetName := "test-statefulset"
	recommendationID := "rec-123"
	oldRevision := "test-statefulset-rev1"
	newRevision := "test-statefulset-rev2"

	expectedGVK := schema.GroupVersionKind{
		Group:   "apps",
		Version: "v1",
		Kind:    kubernetes.StatefulSetKind,
	}

	fakePai := &model.FakePodAutoscalerInternal{
		Namespace: autoscalerNamespace,
		Name:      autoscalerName,
		Spec: &datadoghq.DatadogPodAutoscalerSpec{
			TargetRef: v2.CrossVersionObjectReference{
				Name:       statefulSetName,
				Kind:       expectedGVK.Kind,
				APIVersion: expectedGVK.Group + "/" + expectedGVK.Version,
			},
		},
		TargetGVK:       expectedGVK,
		CurrentReplicas: pointer.Ptr[int32](3),
	}

	// Pods have different controller-revision-hash labels - rollout in progress
	// StatefulSets update pods in reverse ordinal order, so pod-2 is updated first
	pods := []*workloadmeta.KubernetesPod{
		{
			EntityMeta: workloadmeta.EntityMeta{
				Name:      statefulSetName + "-0",
				Namespace: autoscalerNamespace,
				Labels: map[string]string{
					controllerRevisionHashLabel: oldRevision,
				},
				Annotations: map[string]string{
					model.RecommendationIDAnnotation: "old-rec",
				},
			},
			Owners: []workloadmeta.KubernetesPodOwner{
				{Kind: kubernetes.StatefulSetKind, Name: statefulSetName},
			},
		},
		{
			EntityMeta: workloadmeta.EntityMeta{
				Name:      statefulSetName + "-1",
				Namespace: autoscalerNamespace,
				Labels: map[string]string{
					controllerRevisionHashLabel: oldRevision,
				},
				Annotations: map[string]string{
					model.RecommendationIDAnnotation: "old-rec",
				},
			},
			Owners: []workloadmeta.KubernetesPodOwner{
				{Kind: kubernetes.StatefulSetKind, Name: statefulSetName},
			},
		},
		{
			EntityMeta: workloadmeta.EntityMeta{
				Name:      statefulSetName + "-2",
				Namespace: autoscalerNamespace,
				Labels: map[string]string{
					controllerRevisionHashLabel: newRevision, // Updated pod
				},
				Annotations: map[string]string{
					model.RecommendationIDAnnotation: recommendationID,
				},
			},
			Owners: []workloadmeta.KubernetesPodOwner{
				{Kind: kubernetes.StatefulSetKind, Name: statefulSetName},
			},
		},
	}

	podsPerRecomendationID := map[string]int32{
		"old-rec":        2,
		recommendationID: 1, // Partial update
	}

	target := NamespacedPodOwner{
		Namespace: autoscalerNamespace,
		Kind:      kubernetes.StatefulSetKind,
		Name:      statefulSetName,
	}

	autoscalerInternal := fakePai.Build()
	fakeAutoscaler := &datadoghq.DatadogPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      autoscalerName,
			Namespace: autoscalerNamespace,
		},
	}

	result, err := f.controller.syncStatefulSetKind(
		context.Background(),
		fakeAutoscaler,
		&autoscalerInternal,
		datadoghqcommon.DatadogPodAutoscalerAutoUpdateStrategy,
		target,
		expectedGVK,
		recommendationID,
		pods,
		podsPerRecomendationID,
	)

	assert.NoError(t, err)
	assert.Equal(t, autoscaling.ProcessResult{Requeue: true, RequeueAfter: rolloutCheckRequeueDelay}, result)
}

func TestSyncStatefulSetKind_TriggerRollout(t *testing.T) {
	testTime := time.Now()
	f := newVerticalControllerFixture(t, testTime)

	autoscalerNamespace := "default"
	autoscalerName := "test-statefulset-autoscaler"
	statefulSetName := "test-statefulset"
	recommendationID := "rec-123"
	currentRevision := "test-statefulset-rev1"

	expectedGVK := schema.GroupVersionKind{
		Group:   "apps",
		Version: "v1",
		Kind:    kubernetes.StatefulSetKind,
	}

	// Create a fake StatefulSet in the dynamic client
	statefulSet := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "StatefulSet",
			"metadata": map[string]interface{}{
				"name":      statefulSetName,
				"namespace": autoscalerNamespace,
			},
			"spec": map[string]interface{}{
				"replicas": int64(3),
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"annotations": map[string]interface{}{},
					},
				},
			},
		},
	}

	gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"}
	_, err := f.dynamicClient.Resource(gvr).Namespace(autoscalerNamespace).Create(context.Background(), statefulSet, metav1.CreateOptions{})
	assert.NoError(t, err)

	fakePai := &model.FakePodAutoscalerInternal{
		Namespace: autoscalerNamespace,
		Name:      autoscalerName,
		Spec: &datadoghq.DatadogPodAutoscalerSpec{
			TargetRef: v2.CrossVersionObjectReference{
				Name:       statefulSetName,
				Kind:       expectedGVK.Kind,
				APIVersion: expectedGVK.Group + "/" + expectedGVK.Version,
			},
		},
		TargetGVK:       expectedGVK,
		CurrentReplicas: pointer.Ptr[int32](3),
	}

	// No pods have the new recommendation ID - need to trigger rollout
	// All pods have the same controller-revision-hash - no rollout in progress
	pods := []*workloadmeta.KubernetesPod{
		{
			EntityMeta: workloadmeta.EntityMeta{
				Name:      statefulSetName + "-0",
				Namespace: autoscalerNamespace,
				Labels: map[string]string{
					controllerRevisionHashLabel: currentRevision,
				},
				Annotations: map[string]string{
					model.RecommendationIDAnnotation: "old-rec",
				},
			},
			Owners: []workloadmeta.KubernetesPodOwner{
				{Kind: kubernetes.StatefulSetKind, Name: statefulSetName},
			},
		},
		{
			EntityMeta: workloadmeta.EntityMeta{
				Name:      statefulSetName + "-1",
				Namespace: autoscalerNamespace,
				Labels: map[string]string{
					controllerRevisionHashLabel: currentRevision,
				},
				Annotations: map[string]string{
					model.RecommendationIDAnnotation: "old-rec",
				},
			},
			Owners: []workloadmeta.KubernetesPodOwner{
				{Kind: kubernetes.StatefulSetKind, Name: statefulSetName},
			},
		},
		{
			EntityMeta: workloadmeta.EntityMeta{
				Name:      statefulSetName + "-2",
				Namespace: autoscalerNamespace,
				Labels: map[string]string{
					controllerRevisionHashLabel: currentRevision,
				},
				Annotations: map[string]string{
					model.RecommendationIDAnnotation: "old-rec",
				},
			},
			Owners: []workloadmeta.KubernetesPodOwner{
				{Kind: kubernetes.StatefulSetKind, Name: statefulSetName},
			},
		},
	}

	podsPerRecomendationID := map[string]int32{
		"old-rec":        3,
		recommendationID: 0, // No pods have the new recommendation
	}

	target := NamespacedPodOwner{
		Namespace: autoscalerNamespace,
		Kind:      kubernetes.StatefulSetKind,
		Name:      statefulSetName,
	}

	autoscalerInternal := fakePai.Build()
	fakeAutoscaler := &datadoghq.DatadogPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      autoscalerName,
			Namespace: autoscalerNamespace,
		},
	}

	result, err := f.controller.syncStatefulSetKind(
		context.Background(),
		fakeAutoscaler,
		&autoscalerInternal,
		datadoghqcommon.DatadogPodAutoscalerAutoUpdateStrategy,
		target,
		expectedGVK,
		recommendationID,
		pods,
		podsPerRecomendationID,
	)

	assert.NoError(t, err)
	assert.Equal(t, autoscaling.ProcessResult{Requeue: true, RequeueAfter: rolloutCheckRequeueDelay}, result)

	// Verify vertical action was recorded
	assert.NotNil(t, autoscalerInternal.VerticalLastAction())
	assert.Equal(t, recommendationID, autoscalerInternal.VerticalLastAction().Version)
	assert.Equal(t, datadoghqcommon.DatadogPodAutoscalerRolloutTriggeredVerticalActionType, autoscalerInternal.VerticalLastAction().Type)

	// Verify the StatefulSet was patched
	updatedSts, err := f.dynamicClient.Resource(gvr).Namespace(autoscalerNamespace).Get(context.Background(), statefulSetName, metav1.GetOptions{})
	assert.NoError(t, err)

	annotations, found, err := unstructured.NestedStringMap(updatedSts.Object, "spec", "template", "metadata", "annotations")
	assert.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, recommendationID, annotations[model.RecommendationIDAnnotation])
	assert.NotEmpty(t, annotations[model.RolloutTimestampAnnotation])
}

func TestSyncStatefulSetKind_PatchError(t *testing.T) {
	testTime := time.Now()
	f := newVerticalControllerFixture(t, testTime)

	autoscalerNamespace := "default"
	autoscalerName := "test-statefulset-autoscaler"
	statefulSetName := "nonexistent-statefulset"
	recommendationID := "rec-123"
	currentRevision := "nonexistent-statefulset-rev1"

	expectedGVK := schema.GroupVersionKind{
		Group:   "apps",
		Version: "v1",
		Kind:    kubernetes.StatefulSetKind,
	}

	// Don't create the StatefulSet in the dynamic client - patch will fail

	fakePai := &model.FakePodAutoscalerInternal{
		Namespace: autoscalerNamespace,
		Name:      autoscalerName,
		Spec: &datadoghq.DatadogPodAutoscalerSpec{
			TargetRef: v2.CrossVersionObjectReference{
				Name:       statefulSetName,
				Kind:       expectedGVK.Kind,
				APIVersion: expectedGVK.Group + "/" + expectedGVK.Version,
			},
		},
		TargetGVK:       expectedGVK,
		CurrentReplicas: pointer.Ptr[int32](3),
	}

	// All pods have the same controller-revision-hash - no rollout in progress
	pods := []*workloadmeta.KubernetesPod{
		{
			EntityMeta: workloadmeta.EntityMeta{
				Name:      statefulSetName + "-0",
				Namespace: autoscalerNamespace,
				Labels: map[string]string{
					controllerRevisionHashLabel: currentRevision,
				},
			},
			Owners: []workloadmeta.KubernetesPodOwner{
				{Kind: kubernetes.StatefulSetKind, Name: statefulSetName},
			},
		},
	}

	podsPerRecomendationID := map[string]int32{
		recommendationID: 0,
	}

	target := NamespacedPodOwner{
		Namespace: autoscalerNamespace,
		Kind:      kubernetes.StatefulSetKind,
		Name:      statefulSetName,
	}

	autoscalerInternal := fakePai.Build()
	fakeAutoscaler := &datadoghq.DatadogPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      autoscalerName,
			Namespace: autoscalerNamespace,
		},
	}

	// Add a reactor that returns an error for patch operations
	f.dynamicClient.PrependReactor("patch", "statefulsets", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, assert.AnError
	})

	result, err := f.controller.syncStatefulSetKind(
		context.Background(),
		fakeAutoscaler,
		&autoscalerInternal,
		datadoghqcommon.DatadogPodAutoscalerAutoUpdateStrategy,
		target,
		expectedGVK,
		recommendationID,
		pods,
		podsPerRecomendationID,
	)

	assert.Error(t, err)
	assert.Equal(t, autoscaling.Requeue, result)
	assert.NotNil(t, autoscalerInternal.VerticalLastActionError())
}

// TestSyncStatefulSetKind_ExternalRolloutInProgress tests that we correctly detect
// rollouts triggered by external changes (e.g., image updates) by checking
// controller-revision-hash labels, even when all pods have the same recommendation ID.
func TestSyncStatefulSetKind_ExternalRolloutInProgress(t *testing.T) {
	testTime := time.Now()
	f := newVerticalControllerFixture(t, testTime)

	autoscalerNamespace := "default"
	autoscalerName := "test-statefulset-autoscaler"
	statefulSetName := "test-statefulset"
	recommendationID := "rec-123"
	oldRevision := "test-statefulset-rev1"
	newRevision := "test-statefulset-rev2"

	expectedGVK := schema.GroupVersionKind{
		Group:   "apps",
		Version: "v1",
		Kind:    kubernetes.StatefulSetKind,
	}

	fakePai := &model.FakePodAutoscalerInternal{
		Namespace: autoscalerNamespace,
		Name:      autoscalerName,
		Spec: &datadoghq.DatadogPodAutoscalerSpec{
			TargetRef: v2.CrossVersionObjectReference{
				Name:       statefulSetName,
				Kind:       expectedGVK.Kind,
				APIVersion: expectedGVK.Group + "/" + expectedGVK.Version,
			},
		},
		TargetGVK:       expectedGVK,
		CurrentReplicas: pointer.Ptr[int32](3),
	}

	// Scenario: Someone updated the StatefulSet image (external change).
	// All pods still have the OLD recommendation ID (inherited from template),
	// but they have DIFFERENT controller-revision-hash labels because a rollout is in progress.
	// Without checking controller-revision-hash, we would try to trigger another rollout
	// which would interfere with the ongoing one.
	pods := []*workloadmeta.KubernetesPod{
		{
			EntityMeta: workloadmeta.EntityMeta{
				Name:      statefulSetName + "-0",
				Namespace: autoscalerNamespace,
				Labels: map[string]string{
					controllerRevisionHashLabel: oldRevision, // Old revision
				},
				Annotations: map[string]string{
					model.RecommendationIDAnnotation: "old-rec", // Same recommendation
				},
			},
			Owners: []workloadmeta.KubernetesPodOwner{
				{Kind: kubernetes.StatefulSetKind, Name: statefulSetName},
			},
		},
		{
			EntityMeta: workloadmeta.EntityMeta{
				Name:      statefulSetName + "-1",
				Namespace: autoscalerNamespace,
				Labels: map[string]string{
					controllerRevisionHashLabel: oldRevision, // Old revision
				},
				Annotations: map[string]string{
					model.RecommendationIDAnnotation: "old-rec", // Same recommendation
				},
			},
			Owners: []workloadmeta.KubernetesPodOwner{
				{Kind: kubernetes.StatefulSetKind, Name: statefulSetName},
			},
		},
		{
			EntityMeta: workloadmeta.EntityMeta{
				Name:      statefulSetName + "-2",
				Namespace: autoscalerNamespace,
				Labels: map[string]string{
					controllerRevisionHashLabel: newRevision, // New revision - external rollout in progress!
				},
				Annotations: map[string]string{
					model.RecommendationIDAnnotation: "old-rec", // Still has old recommendation
				},
			},
			Owners: []workloadmeta.KubernetesPodOwner{
				{Kind: kubernetes.StatefulSetKind, Name: statefulSetName},
			},
		},
	}

	// All pods have the OLD recommendation ID - without controller-revision-hash check,
	// we would think we need to trigger a rollout
	podsPerRecomendationID := map[string]int32{
		"old-rec":        3,
		recommendationID: 0,
	}

	target := NamespacedPodOwner{
		Namespace: autoscalerNamespace,
		Kind:      kubernetes.StatefulSetKind,
		Name:      statefulSetName,
	}

	autoscalerInternal := fakePai.Build()
	fakeAutoscaler := &datadoghq.DatadogPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      autoscalerName,
			Namespace: autoscalerNamespace,
		},
	}

	result, err := f.controller.syncStatefulSetKind(
		context.Background(),
		fakeAutoscaler,
		&autoscalerInternal,
		datadoghqcommon.DatadogPodAutoscalerAutoUpdateStrategy,
		target,
		expectedGVK,
		recommendationID,
		pods,
		podsPerRecomendationID,
	)

	// Should detect external rollout in progress and NOT trigger our own rollout
	assert.NoError(t, err)
	assert.Equal(t, autoscaling.ProcessResult{Requeue: true, RequeueAfter: rolloutCheckRequeueDelay}, result)
	// Vertical action should NOT be set since we didn't trigger a rollout
	assert.Nil(t, autoscalerInternal.VerticalLastAction())
}
