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
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/record"
	clock "k8s.io/utils/clock/testing"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	workloadpatcher "github.com/DataDog/datadog-agent/pkg/clusteragent/patcher"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
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
			clock:                      fakeClock,
			eventRecorder:              record.NewFakeRecorder(100),
			patchClient:                workloadpatcher.NewPatcher(dynamicClient, nil),
			progressTracker:            newRolloutProgressTracker(),
			client:                     k8sfake.NewSimpleClientset(),
			inPlaceResizeSupported:     func() *bool { b := true; return &b }(),
			inPlaceResizeSupportedTime: fakeClock.Now(),
		},
	}
}

// attachK8sClient creates a fake k8s client and attaches it to the controller.
func (f *verticalControllerFixture) attachK8sClient() *k8sfake.Clientset {
	k8sClient := k8sfake.NewSimpleClientset()
	f.controller.client = k8sClient
	return k8sClient
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
		f.dynamicClient.PrependReactor(
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
	_, _ = f.dynamicClient.Resource(gvr).Namespace(ns).Create(context.Background(), obj, metav1.CreateOptions{})
}

// scalingValWithRequests builds a VerticalScalingValues with both requests and limits set.
func scalingValWithRequests(recID, cpu string) *model.VerticalScalingValues {
	return &model.VerticalScalingValues{
		ResourcesHash: recID,
		ContainerResources: []datadoghqcommon.DatadogPodAutoscalerContainerResources{
			{
				Name:     "c1",
				Requests: corev1.ResourceList{"cpu": resource.MustParse(cpu)},
				Limits:   corev1.ResourceList{"cpu": resource.MustParse(cpu)},
			},
		},
	}
}

// makeDPAWithFallbackDelay builds a DPA with RolloutFallbackDelay set.
func makeDPAWithFallbackDelay(ns, name string, delaySeconds int32) *datadoghq.DatadogPodAutoscaler {
	return &datadoghq.DatadogPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: datadoghq.DatadogPodAutoscalerSpec{
			ApplyPolicy: &datadoghq.DatadogPodAutoscalerApplyPolicy{
				Update: &datadoghqcommon.DatadogPodAutoscalerUpdatePolicy{
					RolloutFallbackDelay: delaySeconds,
				},
			},
		},
	}
}

// makeDPAWithPendingPeriod builds a DPA with the resize pending period and optional last-action time set.
func makeDPAWithPendingPeriod(ns, name string, periodSeconds int32, lastActionTime *time.Time) *datadoghq.DatadogPodAutoscaler {
	dpa := &datadoghq.DatadogPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: datadoghq.DatadogPodAutoscalerSpec{
			ApplyPolicy: &datadoghq.DatadogPodAutoscalerApplyPolicy{
				Update: &datadoghqcommon.DatadogPodAutoscalerUpdatePolicy{
					ResizePendingPeriod: periodSeconds,
				},
			},
		},
	}
	if lastActionTime != nil {
		dpa.Status.Vertical = &datadoghqcommon.DatadogPodAutoscalerVerticalStatus{
			LastAction: &datadoghqcommon.DatadogPodAutoscalerVerticalAction{
				Time: metav1.NewTime(*lastActionTime),
			},
		}
	}
	return dpa
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

// podWithResizeCondition creates a pod with a given pod condition type and reason.
func podWithResizeCondition(name, recID, ownerKind, owner, conditionType, reason string) *workloadmeta.KubernetesPod {
	return podWithResizeConditionAt(name, recID, ownerKind, owner, conditionType, reason, time.Time{})
}

// podWithResizeConditionAt creates a pod with a given condition and a specific LastTransitionTime.
func podWithResizeConditionAt(name, recID, ownerKind, owner, conditionType, reason string, ltt time.Time) *workloadmeta.KubernetesPod {
	p := pod(name, recID, ownerKind, owner)
	p.Conditions = []workloadmeta.KubernetesPodCondition{{Type: conditionType, Reason: reason, LastTransitionTime: ltt}}
	return p
}

// podTerminating creates a pod with a non-nil DeletionTimestamp.
func podTerminating(name, recID, ownerKind, owner string) *workloadmeta.KubernetesPod {
	p := pod(name, recID, ownerKind, owner)
	now := time.Now()
	p.DeletionTimestamp = &now
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

// countEvictions counts eviction create calls on the fake k8s client.
func countEvictions(t *testing.T, actions []k8stesting.Action) int {
	t.Helper()
	count := 0
	for _, a := range actions {
		if a.GetResource().Resource == "pods" && a.GetSubresource() == "eviction" {
			count++
		}
	}
	return count
}

// interceptEvictions prepends a reactor that accepts all pod evictions, preventing the
// default object tracker from rejecting them because the pods don't exist in the fake client.
func interceptEvictions(k8sClient *k8sfake.Clientset) {
	k8sClient.PrependReactor("create", "pods", func(a k8stesting.Action) (bool, runtime.Object, error) {
		if a.GetSubresource() == "eviction" {
			return true, nil, nil
		}
		return false, nil, nil
	})
}

// buildInPlacePAI builds a PodAutoscalerInternal for in-place mode tests.
// strategy controls ApplyPolicy.Update.Strategy; use "" (empty) for in-place (non-TriggerRollout).
func buildInPlacePAI(ns, name string, sv *model.VerticalScalingValues, strategy datadoghqcommon.DatadogPodAutoscalerUpdateStrategy) model.PodAutoscalerInternal {
	return (&model.FakePodAutoscalerInternal{
		Namespace: ns,
		Name:      name,
		Spec: &datadoghq.DatadogPodAutoscalerSpec{
			TargetRef: v2.CrossVersionObjectReference{Name: "target", Kind: kubernetes.DeploymentKind, APIVersion: "apps/v1"},
			ApplyPolicy: &datadoghq.DatadogPodAutoscalerApplyPolicy{
				Update: &datadoghqcommon.DatadogPodAutoscalerUpdatePolicy{
					Strategy: strategy,
				},
			},
		},
		ScalingValues: model.ScalingValues{Vertical: sv},
	}).Build()
}

// --- patchInPlace unit tests ---

// TestPatchInPlace_NeedsPatch_PatchesResources verifies that patchInPlace issues a resize
// subresource patch followed by a metadata annotation patch.
func TestPatchInPlace_NeedsPatch_PatchesResources(t *testing.T) {
	f := newVerticalControllerFixture(t, time.Now())

	patchCallCount := 0
	f.dynamicClient.PrependReactor("patch", "pods", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		patchCallCount++
		return true, &unstructured.Unstructured{}, nil
	})

	// Pod must contain the "c1" container so fromAutoscalerToContainerResourcePatches
	// includes it in the resize patch.
	p := pod("p1", "old", kubernetes.ReplicaSetKind, "rs1")
	p.Containers = []workloadmeta.OrchestratorContainer{{Name: "c1"}}
	ai := buildInPlacePAI("default", "ai", scalingValWithRequests("r1", "500m"), "")

	err := f.controller.patchInPlace(context.Background(), &ai, p, "r1")
	assert.NoError(t, err)
	// Expect two sequential patches: resize subresource, then metadata annotation.
	assert.Equal(t, 2, patchCallCount, "expected resize patch + annotation patch for pod needing update")
}

// runSyncInPlaceMode runs syncInternal against a deployment target using the in-place path.
// It enables the in_place_vertical_scaling config and sets Mode: Auto on the DPA.
func (f *verticalControllerFixture) runSyncInPlaceMode(t *testing.T, dpa *datadoghq.DatadogPodAutoscaler, sv *model.VerticalScalingValues, recommendationID string, pods []*workloadmeta.KubernetesPod) (autoscaling.ProcessResult, error) {
	t.Helper()
	pkgconfigsetup.Datadog().SetWithoutSource("autoscaling.workload.in_place_vertical_scaling.enabled", true)
	t.Cleanup(func() {
		pkgconfigsetup.Datadog().SetWithoutSource("autoscaling.workload.in_place_vertical_scaling.enabled", false)
	})

	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: kubernetes.DeploymentKind}
	target := NamespacedPodOwner{Namespace: "default", Kind: kubernetes.DeploymentKind, Name: "target"}

	if sv == nil {
		sv = scalingValWithRequests(recommendationID, "500m")
	}
	// Config enabled + Strategy: Auto -> in-place path.
	ai := (&model.FakePodAutoscalerInternal{
		Namespace:     "default",
		Name:          "ai",
		ScalingValues: model.ScalingValues{Vertical: sv},
		Spec: &datadoghq.DatadogPodAutoscalerSpec{
			ApplyPolicy: &datadoghq.DatadogPodAutoscalerApplyPolicy{
				Update: &datadoghqcommon.DatadogPodAutoscalerUpdatePolicy{
					Strategy: datadoghqcommon.DatadogPodAutoscalerAutoUpdateStrategy,
				},
			},
		},
	}).Build()

	if dpa == nil {
		dpa = &datadoghq.DatadogPodAutoscaler{ObjectMeta: metav1.ObjectMeta{Name: "ai", Namespace: "default"}}
	}

	return f.controller.syncInternal(
		context.Background(), dpa, &ai, target, gvk, recommendationID,
		pods, map[string]int32{}, map[string]int32{}, buildPodsByResizeStatus(pods, recommendationID),
	)
}

// buildPodsByResizeStatus mirrors the classification done by sync() for test helpers.
func buildPodsByResizeStatus(pods []*workloadmeta.KubernetesPod, recommendationID string) map[PodResizeStatus][]classifiedPod {
	m := make(map[PodResizeStatus][]classifiedPod)
	for _, pod := range pods {
		if pod.DeletionTimestamp != nil {
			continue
		}
		status, ltt := getPodResizeStatus(pod, recommendationID)
		m[status] = append(m[status], classifiedPod{pod: pod, lastTransitionTime: ltt})
	}
	return m
}

// TestSyncInternal_InPlace_Completed_NoRequeue verifies that pods matching the current
// recommendation with no resize conditions are counted as scaled and no requeue is issued.
func TestSyncInternal_InPlace_Completed_NoRequeue(t *testing.T) {
	f := newVerticalControllerFixture(t, time.Now())

	pods := []*workloadmeta.KubernetesPod{
		pod("p1", "r1", kubernetes.ReplicaSetKind, "rs1"),
		pod("p2", "r1", kubernetes.ReplicaSetKind, "rs1"),
	}
	result, err := f.runSyncInPlaceMode(t, nil, nil, "r1", pods)
	assert.NoError(t, err)
	assert.False(t, result.Requeue, "all pods complete -> no requeue")
}

// TestSyncInternal_InPlace_InProgress_Requeues verifies that a pod actively being resized
// causes a requeue.
func TestSyncInternal_InPlace_InProgress_Requeues(t *testing.T) {
	f := newVerticalControllerFixture(t, time.Now())

	pods := []*workloadmeta.KubernetesPod{
		pod("p1", "r1", kubernetes.ReplicaSetKind, "rs1"),
		podWithResizeCondition("p2", "r1", kubernetes.ReplicaSetKind, "rs1",
			kubePodConditionResizeInProgress, ""),
	}
	result, err := f.runSyncInPlaceMode(t, nil, nil, "r1", pods)
	assert.NoError(t, err)
	assert.True(t, result.Requeue)
	assert.Equal(t, inplaceResizeRequeueDelay, result.RequeueAfter)
}

// TestSyncInternal_InPlace_SkipsTerminatingPods verifies that terminating pods are not
// patched and are excluded from the active-pod count.
func TestSyncInternal_InPlace_SkipsTerminatingPods(t *testing.T) {
	f := newVerticalControllerFixture(t, time.Now())

	podPatched := false
	f.dynamicClient.PrependReactor("patch", "pods", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		podPatched = true
		return true, &unstructured.Unstructured{}, nil
	})

	pods := []*workloadmeta.KubernetesPod{podTerminating("p1", "old", kubernetes.ReplicaSetKind, "rs1")}
	result, err := f.runSyncInPlaceMode(t, nil, nil, "r1", pods)
	assert.NoError(t, err)
	assert.False(t, podPatched, "terminating pods must not be patched")
	// All active (non-terminating) pods are zero -> complete -> no requeue.
	assert.False(t, result.Requeue, "no active pods means resize is complete")
}

// TestSyncInternal_InPlace_Infeasible_Evicts verifies that a pod with an infeasible resize
// is evicted.
func TestSyncInternal_InPlace_Infeasible_Evicts(t *testing.T) {
	f := newVerticalControllerFixture(t, time.Now())
	k8sClient := f.attachK8sClient()
	interceptEvictions(k8sClient)

	pods := []*workloadmeta.KubernetesPod{
		podWithResizeCondition("p1", "r1", kubernetes.ReplicaSetKind, "rs1",
			kubePodConditionResizePending, kubePodConditionResizePendingReasonInfeasible),
	}
	_, err := f.runSyncInPlaceMode(t, nil, nil, "r1", pods)
	assert.NoError(t, err)
	assert.Equal(t, 1, countEvictions(t, k8sClient.Actions()))
}

// TestSyncInternal_InPlace_Error_Evicts verifies that a pod with PodResizeInProgress/Error
// is evicted.
func TestSyncInternal_InPlace_Error_Evicts(t *testing.T) {
	f := newVerticalControllerFixture(t, time.Now())
	k8sClient := f.attachK8sClient()
	interceptEvictions(k8sClient)

	pods := []*workloadmeta.KubernetesPod{
		podWithResizeCondition("p1", "r1", kubernetes.ReplicaSetKind, "rs1",
			kubePodConditionResizeInProgress, kubePodConditionResizeInProgressReasonError),
	}
	_, err := f.runSyncInPlaceMode(t, nil, nil, "r1", pods)
	assert.NoError(t, err)
	assert.Equal(t, 1, countEvictions(t, k8sClient.Actions()))
}

// TestSyncInternal_InPlace_Deferred_PeriodElapsed_Evicts verifies a deferred pod is evicted
// once ResizePendingPeriod has elapsed.
func TestSyncInternal_InPlace_Deferred_PeriodElapsed_Evicts(t *testing.T) {
	now := time.Now()
	f := newVerticalControllerFixture(t, now)
	k8sClient := f.attachK8sClient()
	interceptEvictions(k8sClient)

	lastAction := now.Add(-2 * time.Minute)
	dpa := makeDPAWithPendingPeriod("default", "ai", 60, &lastAction)

	pods := []*workloadmeta.KubernetesPod{
		podWithResizeCondition("p1", "r1", kubernetes.ReplicaSetKind, "rs1",
			kubePodConditionResizePending, kubePodConditionResizePendingReasonDeferred),
	}
	_, err := f.runSyncInPlaceMode(t, dpa, nil, "r1", pods)
	assert.NoError(t, err)
	assert.Equal(t, 1, countEvictions(t, k8sClient.Actions()))
}

// TestSyncInternal_InPlace_Deferred_PeriodNotElapsed_NoEviction verifies a deferred pod is
// NOT evicted while ResizePendingPeriod has not elapsed.
func TestSyncInternal_InPlace_Deferred_PeriodNotElapsed_NoEviction(t *testing.T) {
	now := time.Now()
	f := newVerticalControllerFixture(t, now)
	k8sClient := f.attachK8sClient()

	lastAction := now.Add(-30 * time.Second)
	dpa := makeDPAWithPendingPeriod("default", "ai", 60, &lastAction)

	pods := []*workloadmeta.KubernetesPod{
		podWithResizeCondition("p1", "r1", kubernetes.ReplicaSetKind, "rs1",
			kubePodConditionResizePending, kubePodConditionResizePendingReasonDeferred),
	}
	_, err := f.runSyncInPlaceMode(t, dpa, nil, "r1", pods)
	assert.NoError(t, err)
	assert.Equal(t, 0, countEvictions(t, k8sClient.Actions()))
}

// TestSyncInternal_InPlace_PDB_StopsEviction verifies that a PDB-blocked eviction stops
// the loop — only the first pod is attempted, and the sync requeues.
func TestSyncInternal_InPlace_PDB_StopsEviction(t *testing.T) {
	f := newVerticalControllerFixture(t, time.Now())
	k8sClient := f.attachK8sClient()

	// Return 429 for every eviction.
	k8sClient.PrependReactor("create", "pods", func(a k8stesting.Action) (bool, runtime.Object, error) {
		if a.GetSubresource() == "eviction" {
			return true, nil, &k8serrors.StatusError{ErrStatus: metav1.Status{Code: 429, Reason: metav1.StatusReasonTooManyRequests}}
		}
		return false, nil, nil
	})

	// Two infeasible pods; PDB blocks the first -> second should not be attempted.
	pods := []*workloadmeta.KubernetesPod{
		podWithResizeCondition("p1", "r1", kubernetes.ReplicaSetKind, "rs1",
			kubePodConditionResizePending, kubePodConditionResizePendingReasonInfeasible),
		podWithResizeCondition("p2", "r1", kubernetes.ReplicaSetKind, "rs1",
			kubePodConditionResizePending, kubePodConditionResizePendingReasonInfeasible),
	}
	result, err := f.runSyncInPlaceMode(t, nil, nil, "r1", pods)
	assert.NoError(t, err)
	assert.True(t, result.Requeue, "must requeue after PDB-blocked eviction")
	assert.Equal(t, 1, countEvictions(t, k8sClient.Actions()), "only first eviction attempted before PDB stop")
}

// TestSyncInternal_InPlace_FallbackToRollout_WhenStuck verifies that a pod stuck beyond
// RolloutFallbackDelay triggers a rollout instead of eviction.
func TestSyncInternal_InPlace_FallbackToRollout_WhenStuck(t *testing.T) {
	now := time.Now()
	f := newVerticalControllerFixture(t, now)
	f.createTarget("default", "d1", kubernetes.DeploymentKind)

	workloadPatched := false
	f.dynamicClient.PrependReactor("patch", "deployments", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		workloadPatched = true
		return true, &unstructured.Unstructured{Object: map[string]any{
			"metadata": map[string]any{"name": "d1", "namespace": "default"},
		}}, nil
	})

	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: kubernetes.DeploymentKind}
	target := NamespacedPodOwner{Namespace: "default", Kind: kubernetes.DeploymentKind, Name: "d1"}

	ai := (&model.FakePodAutoscalerInternal{
		Namespace: "default",
		Name:      "ai",
		Spec: &datadoghq.DatadogPodAutoscalerSpec{
			TargetRef: v2.CrossVersionObjectReference{Name: "d1", Kind: kubernetes.DeploymentKind, APIVersion: "apps/v1"},
		},
		ScalingValues: model.ScalingValues{Vertical: scalingValWithRequests("r1", "500m")},
	}).Build()

	stuckSince := now.Add(-10 * time.Minute)
	pods := []*workloadmeta.KubernetesPod{
		podWithResizeConditionAt("p1", "r1", kubernetes.ReplicaSetKind, "rs1",
			kubePodConditionResizePending, kubePodConditionResizePendingReasonInfeasible, stuckSince),
	}
	dpa := makeDPAWithFallbackDelay("default", "ai", 300) // 5 minutes

	result, err := f.controller.syncInternal(
		context.Background(), dpa, &ai, target, gvk, "r1",
		pods, map[string]int32{}, map[string]int32{}, buildPodsByResizeStatus(pods, "r1"),
	)
	assert.NoError(t, err)
	assert.True(t, workloadPatched, "rollout must be triggered when pod is stuck beyond RolloutFallbackDelay")
	assert.True(t, result.Requeue)
}

// TestSyncInternal_InPlace_NoFallback_WhenNotStuckLongEnough verifies that a pod stuck
// less than RolloutFallbackDelay is evicted normally without triggering a rollout.
func TestSyncInternal_InPlace_NoFallback_WhenNotStuckLongEnough(t *testing.T) {
	now := time.Now()
	f := newVerticalControllerFixture(t, now)
	k8sClient := f.attachK8sClient()
	interceptEvictions(k8sClient)

	workloadPatched := false
	f.dynamicClient.PrependReactor("patch", "deployments", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		workloadPatched = true
		return true, &unstructured.Unstructured{}, nil
	})

	stuckSince := now.Add(-1 * time.Minute)
	pods := []*workloadmeta.KubernetesPod{
		podWithResizeConditionAt("p1", "r1", kubernetes.ReplicaSetKind, "rs1",
			kubePodConditionResizePending, kubePodConditionResizePendingReasonInfeasible, stuckSince),
	}
	dpa := makeDPAWithFallbackDelay("default", "ai", 300) // 5 minutes

	_, err := f.runSyncInPlaceMode(t, dpa, nil, "r1", pods)
	assert.NoError(t, err)
	assert.False(t, workloadPatched, "no rollout when pod has not been stuck long enough")
	assert.Equal(t, 1, countEvictions(t, k8sClient.Actions()), "eviction must proceed when under the fallback threshold")
}

// TestSyncInternal_TriggerRolloutStrategy_UsesRolloutPath verifies that when
// ApplyPolicy.Update.Strategy is TriggerRollout, syncInternal patches the workload
// (rollout path) rather than individual pods.
func TestSyncInternal_TriggerRolloutStrategy_UsesRolloutPath(t *testing.T) {
	f := newVerticalControllerFixture(t, time.Now())
	f.createTarget("default", "d1", kubernetes.DeploymentKind)

	workloadPatched := false
	f.dynamicClient.PrependReactor("patch", "deployments", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		workloadPatched = true
		return true, &unstructured.Unstructured{Object: map[string]any{
			"metadata": map[string]any{"name": "d1", "namespace": "default"},
		}}, nil
	})

	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: kubernetes.DeploymentKind}
	target := NamespacedPodOwner{Namespace: "default", Kind: kubernetes.DeploymentKind, Name: "d1"}

	ai := (&model.FakePodAutoscalerInternal{
		Namespace: "default",
		Name:      "ai",
		TargetGVK: gvk,
		Spec: &datadoghq.DatadogPodAutoscalerSpec{
			TargetRef: v2.CrossVersionObjectReference{Name: "d1", Kind: kubernetes.DeploymentKind, APIVersion: "apps/v1"},
			ApplyPolicy: &datadoghq.DatadogPodAutoscalerApplyPolicy{
				Update: &datadoghqcommon.DatadogPodAutoscalerUpdatePolicy{
					Strategy: datadoghqcommon.DatadogPodAutoscalerTriggerRolloutUpdateStrategy,
				},
			},
		},
		ScalingValues: model.ScalingValues{Vertical: scalingValWithRequests("r1", "500m")},
	}).Build()

	fakeAutoscaler := &datadoghq.DatadogPodAutoscaler{ObjectMeta: metav1.ObjectMeta{Name: "ai", Namespace: "default"}}
	pods := []*workloadmeta.KubernetesPod{pod("p1", "old", kubernetes.ReplicaSetKind, "rs1")}

	_, err := f.controller.syncInternal(
		context.Background(), fakeAutoscaler, &ai, target, gvk, "r1",
		pods, map[string]int32{"old": 1}, map[string]int32{"rs1": 1},
		buildPodsByResizeStatus(pods, "r1"),
	)
	assert.NoError(t, err)
	assert.True(t, workloadPatched, "TriggerRollout mode must patch the workload, not pods")
}

// TestSyncInternal_ConfigDisabled_NoApplyPolicy_UsesRolloutPath verifies that with the
// config flag disabled (default) and no ApplyPolicy on the DPA, the rollout path is used.
func TestSyncInternal_ConfigDisabled_NoApplyPolicy_UsesRolloutPath(t *testing.T) {
	// Config flag defaults to false — do not set it.
	f := newVerticalControllerFixture(t, time.Now())
	f.createTarget("default", "d1", kubernetes.DeploymentKind)

	workloadPatched := false
	f.dynamicClient.PrependReactor("patch", "deployments", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		workloadPatched = true
		return true, &unstructured.Unstructured{Object: map[string]any{
			"metadata": map[string]any{"name": "d1", "namespace": "default"},
		}}, nil
	})

	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: kubernetes.DeploymentKind}
	target := NamespacedPodOwner{Namespace: "default", Kind: kubernetes.DeploymentKind, Name: "d1"}

	// No ApplyPolicy, config disabled -> rollout.
	ai := (&model.FakePodAutoscalerInternal{
		Namespace: "default",
		Name:      "ai",
		TargetGVK: gvk,
		Spec: &datadoghq.DatadogPodAutoscalerSpec{
			TargetRef: v2.CrossVersionObjectReference{Name: "d1", Kind: kubernetes.DeploymentKind, APIVersion: "apps/v1"},
		},
		ScalingValues: model.ScalingValues{Vertical: scalingValWithRequests("r1", "500m")},
	}).Build()

	fakeAutoscaler := &datadoghq.DatadogPodAutoscaler{ObjectMeta: metav1.ObjectMeta{Name: "ai", Namespace: "default"}}
	pods := []*workloadmeta.KubernetesPod{pod("p1", "old", kubernetes.ReplicaSetKind, "rs1")}

	_, err := f.controller.syncInternal(
		context.Background(), fakeAutoscaler, &ai, target, gvk, "r1",
		pods, map[string]int32{"old": 1}, map[string]int32{"rs1": 1},
		buildPodsByResizeStatus(pods, "r1"),
	)
	assert.NoError(t, err)
	assert.True(t, workloadPatched, "Config disabled + no ApplyPolicy must use rollout path")
}

// TestSyncInternal_ConfigDisabled_AutoStrategy_UsesRolloutPath verifies that with the
// config flag disabled, even a DPA with Strategy: Auto still uses the rollout path.
func TestSyncInternal_ConfigDisabled_AutoStrategy_UsesRolloutPath(t *testing.T) {
	// Config flag defaults to false — do not set it.
	f := newVerticalControllerFixture(t, time.Now())
	f.createTarget("default", "d1", kubernetes.DeploymentKind)

	workloadPatched := false
	f.dynamicClient.PrependReactor("patch", "deployments", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		workloadPatched = true
		return true, &unstructured.Unstructured{Object: map[string]any{
			"metadata": map[string]any{"name": "d1", "namespace": "default"},
		}}, nil
	})

	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: kubernetes.DeploymentKind}
	target := NamespacedPodOwner{Namespace: "default", Kind: kubernetes.DeploymentKind, Name: "d1"}

	// Strategy: Auto but config disabled -> rollout.
	ai := (&model.FakePodAutoscalerInternal{
		Namespace: "default",
		Name:      "ai",
		TargetGVK: gvk,
		Spec: &datadoghq.DatadogPodAutoscalerSpec{
			TargetRef: v2.CrossVersionObjectReference{Name: "d1", Kind: kubernetes.DeploymentKind, APIVersion: "apps/v1"},
			ApplyPolicy: &datadoghq.DatadogPodAutoscalerApplyPolicy{
				Update: &datadoghqcommon.DatadogPodAutoscalerUpdatePolicy{
					Strategy: datadoghqcommon.DatadogPodAutoscalerAutoUpdateStrategy,
				},
			},
		},
		ScalingValues: model.ScalingValues{Vertical: scalingValWithRequests("r1", "500m")},
	}).Build()

	fakeAutoscaler := &datadoghq.DatadogPodAutoscaler{ObjectMeta: metav1.ObjectMeta{Name: "ai", Namespace: "default"}}
	pods := []*workloadmeta.KubernetesPod{pod("p1", "old", kubernetes.ReplicaSetKind, "rs1")}

	_, err := f.controller.syncInternal(
		context.Background(), fakeAutoscaler, &ai, target, gvk, "r1",
		pods, map[string]int32{"old": 1}, map[string]int32{"rs1": 1},
		buildPodsByResizeStatus(pods, "r1"),
	)
	assert.NoError(t, err)
	assert.True(t, workloadPatched, "Config disabled + Strategy: Auto must still use rollout path")
}

// TestSyncInternal_InPlaceEnabled_NoApplyPolicy_UsesInPlacePath verifies that with the
// config flag enabled and no ApplyPolicy on the DPA, in-place is used (Auto strategy is assumed).
func TestSyncInternal_InPlaceEnabled_NoApplyPolicy_UsesInPlacePath(t *testing.T) {
	pkgconfigsetup.Datadog().SetWithoutSource("autoscaling.workload.in_place_vertical_scaling.enabled", true)
	defer pkgconfigsetup.Datadog().SetWithoutSource("autoscaling.workload.in_place_vertical_scaling.enabled", false)

	f := newVerticalControllerFixture(t, time.Now())

	workloadPatched := false
	f.dynamicClient.PrependReactor("patch", "deployments", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		workloadPatched = true
		return true, &unstructured.Unstructured{}, nil
	})

	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: kubernetes.DeploymentKind}
	target := NamespacedPodOwner{Namespace: "default", Kind: kubernetes.DeploymentKind, Name: "d1"}

	ai := (&model.FakePodAutoscalerInternal{
		Namespace: "default",
		Name:      "ai",
		TargetGVK: gvk,
		Spec: &datadoghq.DatadogPodAutoscalerSpec{
			TargetRef: v2.CrossVersionObjectReference{Name: "d1", Kind: kubernetes.DeploymentKind, APIVersion: "apps/v1"},
		},
		ScalingValues: model.ScalingValues{Vertical: scalingValWithRequests("r1", "500m")},
	}).Build()

	fakeAutoscaler := &datadoghq.DatadogPodAutoscaler{ObjectMeta: metav1.ObjectMeta{Name: "ai", Namespace: "default"}}
	pods := []*workloadmeta.KubernetesPod{pod("p1", "r1", kubernetes.ReplicaSetKind, "rs1")}

	_, err := f.controller.syncInternal(
		context.Background(), fakeAutoscaler, &ai, target, gvk, "r1",
		pods, map[string]int32{"r1": 1}, map[string]int32{"rs1": 1},
		buildPodsByResizeStatus(pods, "r1"),
	)
	assert.NoError(t, err)
	assert.False(t, workloadPatched, "Config enabled + nil ApplyPolicy must use in-place path")
}

// TestSyncInternal_InPlaceEnabled_AutoStrategy_UsesInPlacePath verifies that in-place scaling
// is used only when the config flag is enabled AND the DPA explicitly sets Strategy: Auto.
func TestSyncInternal_InPlaceEnabled_AutoStrategy_UsesInPlacePath(t *testing.T) {
	pkgconfigsetup.Datadog().SetWithoutSource("autoscaling.workload.in_place_vertical_scaling.enabled", true)
	defer pkgconfigsetup.Datadog().SetWithoutSource("autoscaling.workload.in_place_vertical_scaling.enabled", false)

	f := newVerticalControllerFixture(t, time.Now())

	workloadPatched := false
	f.dynamicClient.PrependReactor("patch", "deployments", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		workloadPatched = true
		return true, &unstructured.Unstructured{}, nil
	})

	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: kubernetes.DeploymentKind}
	target := NamespacedPodOwner{Namespace: "default", Kind: kubernetes.DeploymentKind, Name: "d1"}

	// Config enabled + Strategy: Auto -> in-place path.
	ai := (&model.FakePodAutoscalerInternal{
		Namespace: "default",
		Name:      "ai",
		TargetGVK: gvk,
		Spec: &datadoghq.DatadogPodAutoscalerSpec{
			TargetRef: v2.CrossVersionObjectReference{Name: "d1", Kind: kubernetes.DeploymentKind, APIVersion: "apps/v1"},
			ApplyPolicy: &datadoghq.DatadogPodAutoscalerApplyPolicy{
				Update: &datadoghqcommon.DatadogPodAutoscalerUpdatePolicy{
					Strategy: datadoghqcommon.DatadogPodAutoscalerAutoUpdateStrategy,
				},
			},
		},
		ScalingValues: model.ScalingValues{Vertical: scalingValWithRequests("r1", "500m")},
	}).Build()

	fakeAutoscaler := &datadoghq.DatadogPodAutoscaler{ObjectMeta: metav1.ObjectMeta{Name: "ai", Namespace: "default"}}
	// Pod already complete -> in-place loop finishes without any patch.
	pods := []*workloadmeta.KubernetesPod{pod("p1", "r1", kubernetes.ReplicaSetKind, "rs1")}

	_, err := f.controller.syncInternal(
		context.Background(), fakeAutoscaler, &ai, target, gvk, "r1",
		pods, map[string]int32{"r1": 1}, map[string]int32{"rs1": 1},
		buildPodsByResizeStatus(pods, "r1"),
	)
	assert.NoError(t, err)
	assert.False(t, workloadPatched, "Config enabled + Strategy: Auto must use in-place path, not rollout")
}
