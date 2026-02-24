// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package workload

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	v2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
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
	dynamicClient *dynamicfake.FakeDynamicClient
	controller    *verticalController
}

func newVerticalControllerFixture(t *testing.T, testTime time.Time) *verticalControllerFixture {
	fakeClock := clock.NewFakeClock(testTime)
	dynamicClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	return &verticalControllerFixture{
		t:             t,
		clock:         fakeClock,
		dynamicClient: dynamicClient,
		controller: &verticalController{
			clock:           fakeClock,
			eventRecorder:   record.NewFakeRecorder(100),
			dynamicClient:   dynamicClient,
			progressTracker: newRolloutProgressTracker(),
		},
	}
}

type verticalTestArgs struct {
	targetKind       string
	targetName       string
	recommendationID string

	pods                    []*workloadmeta.KubernetesPod
	podsPerRecommendationID map[string]int32
	podsPerDirectOwner      map[string]int32 // For Deployments only

	lastAction    *datadoghqcommon.DatadogPodAutoscalerVerticalAction
	scalingValues *model.VerticalScalingValues

	// Control flags
	createTarget bool
	patchError   bool

	// Expectations
	expectPatch         bool
	expectError         bool
	expectActionSet     bool
	expectActionCleared bool
}

func (f *verticalControllerFixture) runSync(args verticalTestArgs) {
	f.t.Helper()
	ns := "default"

	// Determine GVK based on target kind
	var gvk schema.GroupVersionKind
	var resourceName string
	if args.targetKind == kubernetes.RolloutKind {
		gvk = schema.GroupVersionKind{Group: "argoproj.io", Version: "v1alpha1", Kind: args.targetKind}
		resourceName = "rollouts"
	} else {
		gvk = schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: args.targetKind}
		resourceName = strings.ToLower(args.targetKind) + "s"
	}

	if args.createTarget {
		f.createTarget(ns, args.targetName, args.targetKind)
	}

	// Setup patch reactor
	patchCalled := false
	if args.patchError || args.expectPatch {
		var patchErr error
		if args.patchError {
			patchErr = assert.AnError
		}
		f.controller.dynamicClient.(*dynamicfake.FakeDynamicClient).PrependReactor(
			"patch", resourceName, func(action k8stesting.Action) (bool, runtime.Object, error) {
				patchCalled = true
				if patchErr != nil {
					return true, nil, patchErr
				}
				pa := action.(k8stesting.PatchAction)
				return true, &unstructured.Unstructured{Object: map[string]any{
					"apiVersion": "apps/v1",
					"kind":       args.targetKind,
					"metadata":   map[string]any{"name": pa.GetName(), "namespace": pa.GetNamespace()},
				}}, nil
			})
	}

	// Build autoscaler internal
	pai := &model.FakePodAutoscalerInternal{
		Namespace:          ns,
		Name:               args.targetName + "-autoscaler",
		TargetGVK:          gvk,
		CurrentReplicas:    pointer.Ptr[int32](3),
		VerticalLastAction: args.lastAction,
		Spec: &datadoghq.DatadogPodAutoscalerSpec{
			TargetRef: v2.CrossVersionObjectReference{
				Name:       args.targetName,
				Kind:       gvk.Kind,
				APIVersion: "apps/v1",
			},
		},
	}
	if args.scalingValues != nil {
		pai.ScalingValues = model.ScalingValues{Vertical: args.scalingValues}
	}

	autoscalerInternal := pai.Build()
	fakeAutoscaler := &datadoghq.DatadogPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: pai.Name, Namespace: ns},
	}
	target := NamespacedPodOwner{Namespace: ns, Kind: args.targetKind, Name: args.targetName}

	// Execute sync based on target kind
	var err error
	switch args.targetKind {
	case kubernetes.DeploymentKind:
		_, err = f.controller.syncDeploymentKind(
			context.Background(), fakeAutoscaler, &autoscalerInternal, target, gvk,
			args.recommendationID, args.pods, args.podsPerRecommendationID, args.podsPerDirectOwner,
		)
	case kubernetes.RolloutKind:
		_, err = f.controller.syncRolloutKind(
			context.Background(), fakeAutoscaler, &autoscalerInternal, target, gvk,
			args.recommendationID, args.pods, args.podsPerRecommendationID, args.podsPerDirectOwner,
		)
	case kubernetes.StatefulSetKind:
		_, err = f.controller.syncStatefulSetKind(
			context.Background(), fakeAutoscaler, &autoscalerInternal, target, gvk,
			args.recommendationID, args.pods, args.podsPerRecommendationID,
		)
	}

	// Verify expectations
	if args.expectError {
		assert.Error(f.t, err)
	} else {
		assert.NoError(f.t, err)
	}
	assert.Equal(f.t, args.expectPatch, patchCalled, "patch call mismatch")

	if args.expectActionSet {
		assert.NotNil(f.t, autoscalerInternal.VerticalLastAction())
		assert.Equal(f.t, args.recommendationID, autoscalerInternal.VerticalLastAction().Version)
	}
	if args.expectActionCleared {
		assert.Nil(f.t, autoscalerInternal.VerticalLastAction())
	}
}

func (f *verticalControllerFixture) createTarget(ns, name, kind string) {
	var obj *unstructured.Unstructured
	var gvr schema.GroupVersionResource
	switch kind {
	case kubernetes.DeploymentKind:
		obj, _ = autoscaling.ToUnstructured(&appsv1.Deployment{
			TypeMeta:   metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec: appsv1.DeploymentSpec{
				Replicas: pointer.Ptr(int32(3)),
				Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{}}},
			},
		})
		gvr = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	case kubernetes.RolloutKind:
		// Argo Rollout - create as unstructured since we don't have the type
		obj = &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Rollout",
			"metadata":   map[string]any{"name": name, "namespace": ns},
			"spec": map[string]any{
				"replicas": int64(3),
				"template": map[string]any{
					"metadata": map[string]any{"annotations": map[string]any{}},
				},
			},
		}}
		gvr = schema.GroupVersionResource{Group: "argoproj.io", Version: "v1alpha1", Resource: "rollouts"}
	case kubernetes.StatefulSetKind:
		obj, _ = autoscaling.ToUnstructured(&appsv1.StatefulSet{
			TypeMeta:   metav1.TypeMeta{APIVersion: "apps/v1", Kind: "StatefulSet"},
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec: appsv1.StatefulSetSpec{
				Replicas: pointer.Ptr(int32(3)),
				Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{}}},
			},
		})
		gvr = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"}
	}
	_, _ = f.controller.dynamicClient.Resource(gvr).Namespace(ns).Create(context.Background(), obj, metav1.CreateOptions{})
}

// Pod builders
func pod(name, recID, ownerKind, owner string) *workloadmeta.KubernetesPod {
	return &workloadmeta.KubernetesPod{
		EntityMeta: workloadmeta.EntityMeta{
			Name: name, Namespace: "default",
			Annotations: map[string]string{model.RecommendationIDAnnotation: recID},
		},
		Owners: []workloadmeta.KubernetesPodOwner{{Kind: ownerKind, Name: owner, ID: owner}},
	}
}

func podWithRev(name, recID, ownerKind, owner, rev string) *workloadmeta.KubernetesPod {
	p := pod(name, recID, ownerKind, owner)
	p.Labels = map[string]string{controllerRevisionHashLabel: rev}
	return p
}

func podWithLimit(name, recID, ownerKind, owner string, cpu float64) *workloadmeta.KubernetesPod {
	p := pod(name, recID, ownerKind, owner)
	p.Containers = []workloadmeta.OrchestratorContainer{{Name: "c1", Resources: workloadmeta.ContainerResources{CPULimit: &cpu}}}
	return p
}

func podWithRevLimit(name, recID, ownerKind, owner, rev string, cpu float64) *workloadmeta.KubernetesPod {
	p := podWithRev(name, recID, ownerKind, owner, rev)
	p.Containers = []workloadmeta.OrchestratorContainer{{Name: "c1", Resources: workloadmeta.ContainerResources{CPULimit: &cpu}}}
	return p
}

func scalingVal(recID string, cpuLimit string) *model.VerticalScalingValues {
	return &model.VerticalScalingValues{
		ResourcesHash: recID,
		ContainerResources: []datadoghqcommon.DatadogPodAutoscalerContainerResources{
			{Name: "c1", Limits: corev1.ResourceList{"cpu": resource.MustParse(cpuLimit)}},
		},
	}
}

func lastAction(t time.Time, version string) *datadoghqcommon.DatadogPodAutoscalerVerticalAction {
	return &datadoghqcommon.DatadogPodAutoscalerVerticalAction{
		Time: metav1.NewTime(t), Version: version, Type: datadoghqcommon.DatadogPodAutoscalerRolloutTriggeredVerticalActionType,
	}
}

// Deployment tests

func TestDeploymentSyncAllPodsUpdated(t *testing.T) {
	f := newVerticalControllerFixture(t, time.Now())
	f.runSync(verticalTestArgs{
		targetKind:       kubernetes.DeploymentKind,
		targetName:       "d1",
		recommendationID: "r1",
		pods: []*workloadmeta.KubernetesPod{
			pod("p1", "r1", kubernetes.ReplicaSetKind, "rs1"),
			pod("p2", "r1", kubernetes.ReplicaSetKind, "rs1"),
		},
		podsPerRecommendationID: map[string]int32{"r1": 2},
		podsPerDirectOwner:      map[string]int32{"rs1": 2},
		expectActionCleared:     true,
	})
}

func TestDeploymentSyncRolloutInProgress(t *testing.T) {
	f := newVerticalControllerFixture(t, time.Now())
	f.runSync(verticalTestArgs{
		targetKind:       kubernetes.DeploymentKind,
		targetName:       "d1",
		recommendationID: "r1",
		pods: []*workloadmeta.KubernetesPod{
			pod("p1", "old", kubernetes.ReplicaSetKind, "rs1"),
			pod("p2", "r1", kubernetes.ReplicaSetKind, "rs2"),
		},
		podsPerRecommendationID: map[string]int32{"old": 1, "r1": 1},
		podsPerDirectOwner:      map[string]int32{"rs1": 1, "rs2": 1},
	})
}

func TestDeploymentSyncTriggerRollout(t *testing.T) {
	f := newVerticalControllerFixture(t, time.Now())
	f.runSync(verticalTestArgs{
		targetKind:       kubernetes.DeploymentKind,
		targetName:       "d1",
		recommendationID: "r1",
		createTarget:     true,
		pods: []*workloadmeta.KubernetesPod{
			pod("p1", "old", kubernetes.ReplicaSetKind, "rs1"),
		},
		podsPerRecommendationID: map[string]int32{"old": 1, "r1": 0},
		podsPerDirectOwner:      map[string]int32{"rs1": 1},
		expectPatch:             true,
		expectActionSet:         true,
	})
}

func TestDeploymentSyncPatchError(t *testing.T) {
	f := newVerticalControllerFixture(t, time.Now())
	f.runSync(verticalTestArgs{
		targetKind:       kubernetes.DeploymentKind,
		targetName:       "d1",
		recommendationID: "r1",
		pods: []*workloadmeta.KubernetesPod{
			pod("p1", "", kubernetes.ReplicaSetKind, "rs1"),
		},
		podsPerRecommendationID: map[string]int32{"r1": 0},
		podsPerDirectOwner:      map[string]int32{"rs1": 1},
		patchError:              true,
		expectPatch:             true,
		expectError:             true,
	})
}

func TestDeploymentSyncAlreadyTriggeredWaits(t *testing.T) {
	now := time.Now()
	f := newVerticalControllerFixture(t, now)
	f.runSync(verticalTestArgs{
		targetKind:       kubernetes.DeploymentKind,
		targetName:       "d1",
		recommendationID: "r1",
		pods: []*workloadmeta.KubernetesPod{
			pod("p1", "old", kubernetes.ReplicaSetKind, "rs1"),
			pod("p2", "r1", kubernetes.ReplicaSetKind, "rs2"),
		},
		podsPerRecommendationID: map[string]int32{"old": 1, "r1": 1},
		podsPerDirectOwner:      map[string]int32{"rs1": 1, "rs2": 1},
		lastAction:              lastAction(now.Add(-time.Minute), "r1"),
	})
}

func TestDeploymentSyncBypassOnLimitIncrease(t *testing.T) {
	f := newVerticalControllerFixture(t, time.Now())
	f.runSync(verticalTestArgs{
		targetKind:       kubernetes.DeploymentKind,
		targetName:       "d1",
		recommendationID: "r1",
		createTarget:     true,
		pods: []*workloadmeta.KubernetesPod{
			podWithLimit("p1", "old", kubernetes.ReplicaSetKind, "rs1", 25),
			podWithLimit("p2", "old", kubernetes.ReplicaSetKind, "rs2", 25),
		},
		podsPerRecommendationID: map[string]int32{"old": 2, "r1": 0},
		podsPerDirectOwner:      map[string]int32{"rs1": 1, "rs2": 1},
		scalingValues:           scalingVal("r1", "500m"),
		expectPatch:             true,
		expectActionSet:         true,
	})
}

func TestDeploymentSyncBypassRateLimited(t *testing.T) {
	now := time.Now()
	f := newVerticalControllerFixture(t, now)
	f.runSync(verticalTestArgs{
		targetKind:       kubernetes.DeploymentKind,
		targetName:       "d1",
		recommendationID: "r1",
		pods: []*workloadmeta.KubernetesPod{
			podWithLimit("p1", "old", kubernetes.ReplicaSetKind, "rs1", 25),
			podWithLimit("p2", "old", kubernetes.ReplicaSetKind, "rs2", 25),
		},
		podsPerRecommendationID: map[string]int32{"old": 2, "r1": 0},
		podsPerDirectOwner:      map[string]int32{"rs1": 1, "rs2": 1},
		lastAction:              lastAction(now.Add(-2*time.Minute), "old"),
		scalingValues:           scalingVal("r1", "500m"),
	})
}

// StatefulSet tests

func TestStatefulSetSyncAllPodsUpdated(t *testing.T) {
	f := newVerticalControllerFixture(t, time.Now())
	f.runSync(verticalTestArgs{
		targetKind:       kubernetes.StatefulSetKind,
		targetName:       "s1",
		recommendationID: "r1",
		pods: []*workloadmeta.KubernetesPod{
			pod("s1-0", "r1", kubernetes.StatefulSetKind, "s1"),
			pod("s1-1", "r1", kubernetes.StatefulSetKind, "s1"),
		},
		podsPerRecommendationID: map[string]int32{"r1": 2},
		expectActionCleared:     true,
	})
}

func TestStatefulSetSyncRolloutInProgress(t *testing.T) {
	f := newVerticalControllerFixture(t, time.Now())
	f.runSync(verticalTestArgs{
		targetKind:       kubernetes.StatefulSetKind,
		targetName:       "s1",
		recommendationID: "r1",
		pods: []*workloadmeta.KubernetesPod{
			podWithRev("s1-0", "old", kubernetes.StatefulSetKind, "s1", "v1"),
			podWithRev("s1-1", "r1", kubernetes.StatefulSetKind, "s1", "v2"),
		},
		podsPerRecommendationID: map[string]int32{"old": 1, "r1": 1},
	})
}

func TestStatefulSetSyncTriggerRollout(t *testing.T) {
	f := newVerticalControllerFixture(t, time.Now())
	f.runSync(verticalTestArgs{
		targetKind:       kubernetes.StatefulSetKind,
		targetName:       "s1",
		recommendationID: "r1",
		createTarget:     true,
		pods: []*workloadmeta.KubernetesPod{
			podWithRev("s1-0", "old", kubernetes.StatefulSetKind, "s1", "v1"),
		},
		podsPerRecommendationID: map[string]int32{"old": 1, "r1": 0},
		expectPatch:             true,
		expectActionSet:         true,
	})
}

func TestStatefulSetSyncPatchError(t *testing.T) {
	f := newVerticalControllerFixture(t, time.Now())
	f.runSync(verticalTestArgs{
		targetKind:       kubernetes.StatefulSetKind,
		targetName:       "s1",
		recommendationID: "r1",
		pods: []*workloadmeta.KubernetesPod{
			podWithRev("s1-0", "", kubernetes.StatefulSetKind, "s1", "v1"),
		},
		podsPerRecommendationID: map[string]int32{"r1": 0},
		patchError:              true,
		expectPatch:             true,
		expectError:             true,
	})
}

func TestStatefulSetSyncExternalRolloutInProgress(t *testing.T) {
	f := newVerticalControllerFixture(t, time.Now())
	f.runSync(verticalTestArgs{
		targetKind:       kubernetes.StatefulSetKind,
		targetName:       "s1",
		recommendationID: "r1",
		pods: []*workloadmeta.KubernetesPod{
			podWithRev("s1-0", "old", kubernetes.StatefulSetKind, "s1", "v1"),
			podWithRev("s1-1", "old", kubernetes.StatefulSetKind, "s1", "v1"),
			podWithRev("s1-2", "old", kubernetes.StatefulSetKind, "s1", "v2"),
		},
		podsPerRecommendationID: map[string]int32{"old": 3, "r1": 0},
	})
}

func TestStatefulSetSyncBypassOnLimitIncrease(t *testing.T) {
	f := newVerticalControllerFixture(t, time.Now())
	f.runSync(verticalTestArgs{
		targetKind:       kubernetes.StatefulSetKind,
		targetName:       "s1",
		recommendationID: "r1",
		createTarget:     true,
		pods: []*workloadmeta.KubernetesPod{
			podWithRevLimit("s1-0", "old", kubernetes.StatefulSetKind, "s1", "v1", 25),
			podWithRevLimit("s1-1", "old", kubernetes.StatefulSetKind, "s1", "v2", 25),
		},
		podsPerRecommendationID: map[string]int32{"old": 2, "r1": 0},
		scalingValues:           scalingVal("r1", "500m"),
		expectPatch:             true,
		expectActionSet:         true,
	})
}

func TestStatefulSetSyncAlreadyTriggeredWaits(t *testing.T) {
	now := time.Now()
	f := newVerticalControllerFixture(t, now)
	f.runSync(verticalTestArgs{
		targetKind:       kubernetes.StatefulSetKind,
		targetName:       "s1",
		recommendationID: "r1",
		pods: []*workloadmeta.KubernetesPod{
			podWithRev("s1-0", "old", kubernetes.StatefulSetKind, "s1", "v1"),
			podWithRev("s1-1", "r1", kubernetes.StatefulSetKind, "s1", "v2"),
		},
		podsPerRecommendationID: map[string]int32{"old": 1, "r1": 1},
		lastAction:              lastAction(now.Add(-time.Minute), "r1"),
	})
}

func TestStatefulSetSyncBypassRateLimited(t *testing.T) {
	now := time.Now()
	f := newVerticalControllerFixture(t, now)
	f.runSync(verticalTestArgs{
		targetKind:       kubernetes.StatefulSetKind,
		targetName:       "s1",
		recommendationID: "r1",
		pods: []*workloadmeta.KubernetesPod{
			podWithRevLimit("s1-0", "old", kubernetes.StatefulSetKind, "s1", "v1", 25),
			podWithRevLimit("s1-1", "old", kubernetes.StatefulSetKind, "s1", "v2", 25),
		},
		podsPerRecommendationID: map[string]int32{"old": 2, "r1": 0},
		lastAction:              lastAction(now.Add(-2*time.Minute), "old"),
		scalingValues:           scalingVal("r1", "500m"),
	})
}

// Argo Rollout tests
// Single test because it's re-using the same logic as the Deployment test.
func TestRolloutSyncTriggerRollout(t *testing.T) {
	f := newVerticalControllerFixture(t, time.Now())
	f.runSync(verticalTestArgs{
		targetKind:       kubernetes.RolloutKind,
		targetName:       "rollout1",
		recommendationID: "r1",
		createTarget:     true,
		pods: []*workloadmeta.KubernetesPod{
			pod("p1", "old", kubernetes.ReplicaSetKind, "rs1"),
			pod("p2", "old", kubernetes.ReplicaSetKind, "rs1"),
		},
		podsPerRecommendationID: map[string]int32{"old": 2, "r1": 0},
		podsPerDirectOwner:      map[string]int32{"rs1": 2},
		expectPatch:             true,
		expectActionSet:         true,
	})
}
