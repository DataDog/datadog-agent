// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package profile

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func newTestSyncer() (*AutoscalerSyncer, *autoscaling.Store[model.PodAutoscalerProfileInternal], *autoscaling.Store[model.PodAutoscalerInternal]) {
	profileStore := autoscaling.NewStore[model.PodAutoscalerProfileInternal]()
	dpaStore := autoscaling.NewStore[model.PodAutoscalerInternal]()
	s := &AutoscalerSyncer{
		profileStore: profileStore,
		dpaStore:     dpaStore,
		isLeader:     func() bool { return true },
		dpaOwnership: make(map[string]string),
		reconcileCh:  make(chan struct{}, 1),
	}
	return s, profileStore, dpaStore
}

func testRef(ns, name string) model.NamespacedObjectReference {
	return model.NamespacedObjectReference{
		GroupKind: schema.GroupKind{Group: "apps", Kind: "Deployment"},
		Version:   "v1",
		Namespace: ns,
		Name:      name,
	}
}

func testProfileWithWorkloads(name string, workloads []model.NamespacedObjectReference) model.PodAutoscalerProfileInternal {
	maxReplicas := int32(10)
	tmpl := datadoghq.DatadogPodAutoscalerTemplate{
		Objectives: []datadoghqcommon.DatadogPodAutoscalerObjective{
			{
				Type: datadoghqcommon.DatadogPodAutoscalerPodResourceObjectiveType,
				PodResource: &datadoghqcommon.DatadogPodAutoscalerPodResourceObjective{
					Name: "cpu",
					Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
						Type:        datadoghqcommon.DatadogPodAutoscalerUtilizationObjectiveValueType,
						Utilization: int32Ptr(80),
					},
				},
			},
		},
		Constraints: &datadoghqcommon.DatadogPodAutoscalerConstraints{
			MaxReplicas: &maxReplicas,
		},
	}

	pi, _ := model.NewPodAutoscalerProfileInternal(&datadoghq.DatadogPodAutoscalerClusterProfile{
		ObjectMeta: metav1.ObjectMeta{Name: name, Generation: 1},
		Spec:       datadoghq.DatadogPodAutoscalerProfileSpec{Template: tmpl},
	})

	if len(workloads) > 0 {
		pi.UpdateWorkloads(workloads)
	}
	return pi
}

func int32Ptr(v int32) *int32 { return &v }

func TestAutoscalerSyncerCreatesDPA(t *testing.T) {
	s, profileStore, dpaStore := newTestSyncer()

	ref := testRef("prod", "web-app")
	prof := testProfileWithWorkloads("high-cpu", []model.NamespacedObjectReference{ref})
	profileStore.Set("high-cpu", prof, "test")

	s.reconcile()

	workloads := prof.Workloads()
	require.Len(t, workloads, 1)

	for dpaKey := range workloads {
		pai, found := dpaStore.Get(dpaKey)
		require.True(t, found, "Expected DPA store entry %s", dpaKey)
		assert.Equal(t, "high-cpu", pai.ProfileName())
		assert.True(t, pai.IsProfileManaged())
		assert.Equal(t, "prod", pai.Namespace())
		assert.Equal(t, "Deployment", pai.Spec().TargetRef.Kind)
		assert.Equal(t, "web-app", pai.Spec().TargetRef.Name)
		assert.Equal(t, "apps/v1", pai.Spec().TargetRef.APIVersion)
		assert.Equal(t, datadoghqcommon.DatadogPodAutoscalerLocalOwner, pai.Spec().Owner)
		assert.Equal(t, "high-cpu", s.dpaOwnership[dpaKey])
	}
}

func TestAutoscalerSyncerUpdateDPAOnTemplateChange(t *testing.T) {
	s, profileStore, dpaStore := newTestSyncer()

	ref := testRef("prod", "web-app")
	prof := testProfileWithWorkloads("high-cpu", []model.NamespacedObjectReference{ref})
	profileStore.Set("high-cpu", prof, "test")

	s.reconcile()

	// Now update the profile template.
	newMax := int32(20)
	newTmpl := datadoghq.DatadogPodAutoscalerTemplate{
		Objectives: prof.Template().Objectives,
		Constraints: &datadoghqcommon.DatadogPodAutoscalerConstraints{
			MaxReplicas: &newMax,
		},
	}
	updatedProf, _ := model.NewPodAutoscalerProfileInternal(&datadoghq.DatadogPodAutoscalerClusterProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "high-cpu", Generation: 2},
		Spec:       datadoghq.DatadogPodAutoscalerProfileSpec{Template: newTmpl},
	})
	updatedProf.UpdateWorkloads([]model.NamespacedObjectReference{ref})
	profileStore.Set("high-cpu", updatedProf, "test")

	s.reconcile()

	for dpaKey := range updatedProf.Workloads() {
		pai, found := dpaStore.Get(dpaKey)
		require.True(t, found)
		assert.Equal(t, &newMax, pai.Spec().Constraints.MaxReplicas)
	}
}

func TestAutoscalerSyncerMarksDPADeletedWhenWorkloadRemoved(t *testing.T) {
	s, profileStore, dpaStore := newTestSyncer()

	ref := testRef("prod", "web-app")
	prof := testProfileWithWorkloads("high-cpu", []model.NamespacedObjectReference{ref})
	profileStore.Set("high-cpu", prof, "test")

	s.reconcile()

	var dpaKey string
	for k := range prof.Workloads() {
		dpaKey = k
	}
	_, found := dpaStore.Get(dpaKey)
	require.True(t, found, "DPA should exist before removal")

	// Remove workloads from profile.
	prof.UpdateWorkloads(nil)
	profileStore.Set("high-cpu", prof, "test")

	s.reconcile()

	pai, found := dpaStore.Get(dpaKey)
	require.True(t, found)
	assert.True(t, pai.Deleted(), "DPA should be marked deleted when workload ref is removed")
	assert.Empty(t, s.dpaOwnership, "Ownership should be cleared")
}

func TestAutoscalerSyncerMarksDPADeletedWhenProfileDeleted(t *testing.T) {
	s, profileStore, dpaStore := newTestSyncer()

	ref := testRef("prod", "web-app")
	prof := testProfileWithWorkloads("high-cpu", []model.NamespacedObjectReference{ref})
	profileStore.Set("high-cpu", prof, "test")

	s.reconcile()

	var dpaKey string
	for k := range prof.Workloads() {
		dpaKey = k
	}

	// Delete the profile.
	profileStore.Delete("high-cpu", "test")

	s.reconcile()

	pai, found := dpaStore.Get(dpaKey)
	require.True(t, found)
	assert.True(t, pai.Deleted(), "DPA should be marked deleted when profile is deleted")
}

func TestAutoscalerSyncerLabelChangeNoDeleteCreate(t *testing.T) {
	s, profileStore, dpaStore := newTestSyncer()

	ref := testRef("prod", "web-app")
	profA := testProfileWithWorkloads("profile-a", []model.NamespacedObjectReference{ref})
	profB := testProfileWithWorkloads("profile-b", nil)
	profileStore.Set("profile-a", profA, "test")
	profileStore.Set("profile-b", profB, "test")

	s.reconcile()

	var dpaKey string
	for k := range profA.Workloads() {
		dpaKey = k
	}
	paiBeforeSwitch, found := dpaStore.Get(dpaKey)
	require.True(t, found)
	assert.Equal(t, "profile-a", paiBeforeSwitch.ProfileName())

	// Simulate label change: workload moves from profile-a to profile-b.
	profA.UpdateWorkloads(nil)
	profB.UpdateWorkloads([]model.NamespacedObjectReference{ref})
	profileStore.Set("profile-a", profA, "test")
	profileStore.Set("profile-b", profB, "test")

	s.reconcile()

	paiAfterSwitch, found := dpaStore.Get(dpaKey)
	require.True(t, found)
	assert.Equal(t, "profile-b", paiAfterSwitch.ProfileName(), "Profile should switch without delete/create")
	assert.False(t, paiAfterSwitch.Deleted(), "DPA should NOT be deleted on label change")
	assert.Equal(t, "profile-b", s.dpaOwnership[dpaKey])
}

func TestAutoscalerSyncerConflictWithUserDPA(t *testing.T) {
	s, profileStore, dpaStore := newTestSyncer()

	ref := testRef("prod", "web-app")
	prof := testProfileWithWorkloads("high-cpu", []model.NamespacedObjectReference{ref})
	profileStore.Set("high-cpu", prof, "test")

	// User creates a DPA targeting the same workload.
	userDPA := model.FakePodAutoscalerInternal{
		Namespace: "prod",
		Name:      "user-dpa",
		Spec: &datadoghq.DatadogPodAutoscalerSpec{
			TargetRef: autoscalingv2.CrossVersionObjectReference{
				Kind: "Deployment", Name: "web-app", APIVersion: "apps/v1",
			},
			Owner: datadoghqcommon.DatadogPodAutoscalerLocalOwner,
		},
	}.Build()
	dpaStore.Set("prod/user-dpa", userDPA, "dpa-c")

	s.reconcile()

	for dpaKey := range prof.Workloads() {
		_, found := dpaStore.Get(dpaKey)
		assert.False(t, found, "Should NOT create profile-managed DPA when user DPA conflicts")
	}
	assert.Empty(t, s.dpaOwnership)
}

func TestAutoscalerSyncerConflictRemovesExistingProfileDPA(t *testing.T) {
	s, profileStore, dpaStore := newTestSyncer()

	ref := testRef("prod", "web-app")
	prof := testProfileWithWorkloads("high-cpu", []model.NamespacedObjectReference{ref})
	profileStore.Set("high-cpu", prof, "test")

	// First reconcile creates the DPA.
	s.reconcile()

	var dpaKey string
	for k := range prof.Workloads() {
		dpaKey = k
	}
	_, found := dpaStore.Get(dpaKey)
	require.True(t, found, "DPA should exist after first reconcile")

	// Now user creates a conflicting DPA.
	userDPA := model.FakePodAutoscalerInternal{
		Namespace: "prod",
		Name:      "user-dpa",
		Spec: &datadoghq.DatadogPodAutoscalerSpec{
			TargetRef: autoscalingv2.CrossVersionObjectReference{
				Kind: "Deployment", Name: "web-app", APIVersion: "apps/v1",
			},
			Owner: datadoghqcommon.DatadogPodAutoscalerLocalOwner,
		},
	}.Build()
	dpaStore.Set("prod/user-dpa", userDPA, "dpa-c")

	s.reconcile()

	pai, found := dpaStore.Get(dpaKey)
	require.True(t, found)
	assert.True(t, pai.Deleted(), "Existing profile-managed DPA should be marked deleted when user DPA conflicts")
	assert.Empty(t, s.dpaOwnership)
}

func TestAutoscalerSyncerMultipleWorkloads(t *testing.T) {
	s, profileStore, dpaStore := newTestSyncer()

	refs := []model.NamespacedObjectReference{
		testRef("prod", "web-1"),
		testRef("prod", "web-2"),
		testRef("staging", "web-3"),
	}
	prof := testProfileWithWorkloads("high-cpu", refs)
	profileStore.Set("high-cpu", prof, "test")

	s.reconcile()

	for dpaKey, ref := range prof.Workloads() {
		pai, found := dpaStore.Get(dpaKey)
		require.True(t, found, "Expected DPA for %s", ref.Name)
		assert.Equal(t, "high-cpu", pai.ProfileName())
		assert.Equal(t, ref.Name, pai.Spec().TargetRef.Name)
		assert.Equal(t, ref.Namespace, pai.Namespace())
	}
	assert.Len(t, s.dpaOwnership, 3)
}

func TestAutoscalerSyncerInvalidProfileMarksExistingDPAsDeleted(t *testing.T) {
	s, profileStore, dpaStore := newTestSyncer()

	ref := testRef("prod", "web-app")
	prof := testProfileWithWorkloads("high-cpu", []model.NamespacedObjectReference{ref})
	profileStore.Set("high-cpu", prof, "test")

	s.reconcile()

	var dpaKey string
	for k := range prof.Workloads() {
		dpaKey = k
	}

	// Make the profile invalid by replacing with a freshly-constructed invalid one.
	invalidProf, _ := model.NewPodAutoscalerProfileInternal(&datadoghq.DatadogPodAutoscalerClusterProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "high-cpu", Generation: 2},
		Spec: datadoghq.DatadogPodAutoscalerProfileSpec{
			Template: datadoghq.DatadogPodAutoscalerTemplate{
				Constraints: &datadoghqcommon.DatadogPodAutoscalerConstraints{
					MinReplicas: pointer.Ptr[int32](10),
					MaxReplicas: pointer.Ptr[int32](2),
				},
			},
		},
	})
	invalidProf.UpdateWorkloads([]model.NamespacedObjectReference{ref})
	profileStore.Set("high-cpu", invalidProf, "test")

	s.reconcile()

	pai, found := dpaStore.Get(dpaKey)
	require.True(t, found)
	assert.True(t, pai.Deleted(), "DPA should be marked deleted for invalid profile")
}

func TestAutoscalerSyncerIdempotent(t *testing.T) {
	s, profileStore, dpaStore := newTestSyncer()

	ref := testRef("prod", "web-app")
	prof := testProfileWithWorkloads("high-cpu", []model.NamespacedObjectReference{ref})
	profileStore.Set("high-cpu", prof, "test")

	s.reconcile()
	s.reconcile()
	s.reconcile()

	var dpaKey string
	for k := range prof.Workloads() {
		dpaKey = k
	}
	pai, found := dpaStore.Get(dpaKey)
	require.True(t, found)
	assert.False(t, pai.Deleted())
	assert.Equal(t, "high-cpu", pai.ProfileName())
}

func TestAutoscalerSyncerPreservesExistingDPAState(t *testing.T) {
	s, profileStore, dpaStore := newTestSyncer()

	ref := testRef("prod", "web-app")
	prof := testProfileWithWorkloads("high-cpu", []model.NamespacedObjectReference{ref})
	profileStore.Set("high-cpu", prof, "test")

	s.reconcile()

	var dpaKey string
	for k := range prof.Workloads() {
		dpaKey = k
	}

	// Simulate the DPA controller setting generation.
	pai, _, unlock := dpaStore.LockRead(dpaKey, false)
	pai.SetGeneration(5)
	dpaStore.UnlockSet(dpaKey, pai, "dpa-c")
	_ = unlock

	// Run reconcile again — should not reset the generation.
	s.reconcile()

	pai2, found := dpaStore.Get(dpaKey)
	require.True(t, found)
	assert.Equal(t, int64(5), pai2.Generation(), "Generation should be preserved across reconciles")
}

func TestAutoscalerSyncerWaitsForReadyDeps(t *testing.T) {
	profileStore := autoscaling.NewStore[model.PodAutoscalerProfileInternal]()
	dpaStore := autoscaling.NewStore[model.PodAutoscalerInternal]()

	ref := testRef("prod", "web-app")
	prof := testProfileWithWorkloads("high-cpu", []model.NamespacedObjectReference{ref})
	profileStore.Set("high-cpu", prof, "test")

	var ready atomic.Bool
	s := NewAutoscalerSyncer(profileStore, dpaStore, func() bool { return true }, ready.Load)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var runReturned atomic.Bool
	go func() {
		s.Run(ctx)
		runReturned.Store(true)
	}()

	// Give the syncer time to potentially reconcile (it shouldn't).
	time.Sleep(200 * time.Millisecond)
	for dpaKey := range prof.Workloads() {
		_, found := dpaStore.Get(dpaKey)
		assert.False(t, found, "DPA should not be created before deps are ready")
	}

	// Signal readiness — the syncer should reconcile.
	ready.Store(true)
	dpaKeys := make([]string, 0, len(prof.Workloads()))
	for k := range prof.Workloads() {
		dpaKeys = append(dpaKeys, k)
	}
	assert.Eventually(t, func() bool {
		_, found := dpaStore.Get(dpaKeys[0])
		return found
	}, 5*time.Second, 50*time.Millisecond, "Syncer should reconcile after deps become ready")

	cancel()
	assert.Eventually(t, func() bool {
		return runReturned.Load()
	}, 2*time.Second, 50*time.Millisecond, "Run should return after context is cancelled")
}

func TestAutoscalerSyncerRebuildOwnershipCleansOrphans(t *testing.T) {
	profileStore := autoscaling.NewStore[model.PodAutoscalerProfileInternal]()
	dpaStore := autoscaling.NewStore[model.PodAutoscalerInternal]()

	// Simulate restart: the DPA store already has a profile-managed DPA
	// (loaded by the DPA controller from K8s), but the workload no longer
	// has the profile label, so the profile has empty workloads.
	prof := testProfileWithWorkloads("high-cpu", nil) // no workloads
	profileStore.Set("high-cpu", prof, "test")

	targetRef := autoscalingv2.CrossVersionObjectReference{
		Kind: "Deployment", Name: "web-app", APIVersion: "apps/v1",
	}
	orphanDPA := model.NewPodAutoscalerFromProfile("prod", "web-app-9526aeb3", "high-cpu", prof.Template(), targetRef, prof.TemplateHash(), "")
	dpaStore.Set("prod/web-app-9526aeb3", orphanDPA, "dpa-c")

	s := &AutoscalerSyncer{
		profileStore: profileStore,
		dpaStore:     dpaStore,
		isLeader:     func() bool { return true },
		dpaOwnership: make(map[string]string),
		reconcileCh:  make(chan struct{}, 1),
	}

	// rebuildOwnership should discover the orphaned DPA.
	s.rebuildOwnership()
	assert.Equal(t, map[string]string{"prod/web-app-9526aeb3": "high-cpu"}, s.dpaOwnership)

	// The first reconcile should mark it deleted since desired is empty.
	s.reconcile()

	pai, found := dpaStore.Get("prod/web-app-9526aeb3")
	require.True(t, found)
	assert.True(t, pai.Deleted(), "Orphaned DPA should be marked deleted after ownership rebuild + reconcile")
	assert.Empty(t, s.dpaOwnership, "Ownership should be cleared after deletion")
}

func TestAutoscalerSyncerRebuildOwnershipKeepsActiveDPAs(t *testing.T) {
	profileStore := autoscaling.NewStore[model.PodAutoscalerProfileInternal]()
	dpaStore := autoscaling.NewStore[model.PodAutoscalerInternal]()

	// Simulate normal restart: workload still has the label, everything matches.
	ref := testRef("prod", "web-app")
	prof := testProfileWithWorkloads("high-cpu", []model.NamespacedObjectReference{ref})
	profileStore.Set("high-cpu", prof, "test")

	var dpaKey string
	for k := range prof.Workloads() {
		dpaKey = k
	}

	targetRef := autoscalingv2.CrossVersionObjectReference{
		Kind: "Deployment", Name: "web-app", APIVersion: "apps/v1",
	}
	_, dpaName, _ := cache.SplitMetaNamespaceKey(dpaKey)
	existingDPA := model.NewPodAutoscalerFromProfile("prod", dpaName, "high-cpu", prof.Template(), targetRef, prof.TemplateHash(), "")
	dpaStore.Set(dpaKey, existingDPA, "dpa-c")

	s := &AutoscalerSyncer{
		profileStore: profileStore,
		dpaStore:     dpaStore,
		isLeader:     func() bool { return true },
		dpaOwnership: make(map[string]string),
		reconcileCh:  make(chan struct{}, 1),
	}

	s.rebuildOwnership()
	require.Len(t, s.dpaOwnership, 1)

	s.reconcile()

	pai, found := dpaStore.Get(dpaKey)
	require.True(t, found)
	assert.False(t, pai.Deleted(), "Active DPA should NOT be deleted after rebuild + reconcile")
	assert.Equal(t, "high-cpu", s.dpaOwnership[dpaKey])
}

func testProfileWithWorkloadsAndAnnotations(name string, workloads []model.NamespacedObjectReference, annotations map[string]string) model.PodAutoscalerProfileInternal {
	maxReplicas := int32(10)
	tmpl := datadoghq.DatadogPodAutoscalerTemplate{
		Constraints: &datadoghqcommon.DatadogPodAutoscalerConstraints{
			MaxReplicas: &maxReplicas,
		},
	}

	pi, _ := model.NewPodAutoscalerProfileInternal(&datadoghq.DatadogPodAutoscalerClusterProfile{
		ObjectMeta: metav1.ObjectMeta{Name: name, Generation: 1, Annotations: annotations},
		Spec:       datadoghq.DatadogPodAutoscalerProfileSpec{Template: tmpl},
	})

	if len(workloads) > 0 {
		pi.UpdateWorkloads(workloads)
	}
	return pi
}

func TestAutoscalerSyncerBurstableAnnotationPropagatedToDPA(t *testing.T) {
	s, profileStore, dpaStore := newTestSyncer()

	ref := testRef("prod", "web-app")
	prof := testProfileWithWorkloadsAndAnnotations("high-cpu", []model.NamespacedObjectReference{ref},
		map[string]string{model.PreviewAnnotationKey: `{"burstable":true}`})
	profileStore.Set("high-cpu", prof, "test")

	s.reconcile()

	for dpaKey := range prof.Workloads() {
		pai, found := dpaStore.Get(dpaKey)
		require.True(t, found)
		assert.True(t, pai.IsBurstable(), "generated DPA should be burstable when profile carries the annotation")
		assert.Equal(t, `{"burstable":true}`, pai.PreviewAnnotation(),
			"preview annotation should be forwarded from profile to generated DPA")
		// Hash must differ from a non-burstable profile (annotation value is included in hash input)
		nonBurstableProf := testProfileWithWorkloads("high-cpu", []model.NamespacedObjectReference{testRef("prod", "web-app")})
		assert.NotEqual(t, nonBurstableProf.TemplateHash(), pai.DesiredProfileTemplateHash(),
			"burstable profile hash must differ from non-burstable profile hash")
	}
}

func TestAutoscalerSyncerBurstableHashChangeTriggersUpdate(t *testing.T) {
	s, profileStore, dpaStore := newTestSyncer()

	ref := testRef("prod", "web-app")

	// First reconcile: burstable=false
	prof := testProfileWithWorkloads("high-cpu", []model.NamespacedObjectReference{ref})
	profileStore.Set("high-cpu", prof, "test")
	s.reconcile()

	var dpaKey string
	for k := range prof.Workloads() {
		dpaKey = k
	}
	pai, found := dpaStore.Get(dpaKey)
	require.True(t, found)
	assert.False(t, pai.IsBurstable())

	// Second reconcile: burstable=true — hash changes, update must be triggered
	burstableProf := testProfileWithWorkloadsAndAnnotations("high-cpu", []model.NamespacedObjectReference{ref},
		map[string]string{model.PreviewAnnotationKey: `{"burstable":true}`})
	profileStore.Set("high-cpu", burstableProf, "test")
	s.reconcile()

	pai, found = dpaStore.Get(dpaKey)
	require.True(t, found)
	assert.True(t, pai.IsBurstable(), "DPA should become burstable after profile annotation is added")

	// Third reconcile: burstable removed — hash reverts, update must be triggered again
	nonBurstableProf := testProfileWithWorkloads("high-cpu", []model.NamespacedObjectReference{ref})
	profileStore.Set("high-cpu", nonBurstableProf, "test")
	s.reconcile()

	pai, found = dpaStore.Get(dpaKey)
	require.True(t, found)
	assert.False(t, pai.IsBurstable(), "DPA should revert to non-burstable when annotation is removed from profile")
}

// TestAutoscalerSyncerOrphanByLabelRemoval verifies that when a customer
// removes the profile label from a DPA K8s object (simulated here by the DPA
// controller clearing profileName in the store), the syncer:
//   - drops the DPA from dpaOwnership
//   - does NOT mark the DPA deleted
//   - does NOT create a new DPA for the same workload
func TestAutoscalerSyncerOrphanByLabelRemoval(t *testing.T) {
	s, profileStore, dpaStore := newTestSyncer()

	ref := testRef("prod", "web-app")
	prof := testProfileWithWorkloads("high-cpu", []model.NamespacedObjectReference{ref})
	profileStore.Set("high-cpu", prof, "test")

	// Initial reconcile: syncer creates the DPA and owns it.
	s.reconcile()

	var dpaKey string
	for k := range prof.Workloads() {
		dpaKey = k
	}
	pai, found := dpaStore.Get(dpaKey)
	require.True(t, found)
	assert.True(t, pai.IsProfileManaged())
	assert.Equal(t, "high-cpu", s.dpaOwnership[dpaKey])

	// Simulate the DPA controller clearing profileName after the customer
	// removed the profile label from the K8s object.
	pai, _, unlock := dpaStore.LockRead(dpaKey, false)
	pai.SetProfileName("")
	dpaStore.UnlockSet(dpaKey, pai, "dpa-c")
	_ = unlock

	// Next reconcile: syncer should orphan the DPA.
	s.reconcile()

	pai, found = dpaStore.Get(dpaKey)
	require.True(t, found, "Orphaned DPA should still exist in the store")
	assert.False(t, pai.Deleted(), "Orphaned DPA should NOT be marked deleted")
	assert.False(t, pai.IsProfileManaged(), "Orphaned DPA should no longer be profile-managed")
	assert.Empty(t, s.dpaOwnership, "Orphaned DPA should be removed from ownership")
}
