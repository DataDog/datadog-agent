// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package gpuallocation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

// fakeStore implements just enough of workloadmeta.Component for the backends.
type fakeStore struct {
	workloadmeta.Component
	claims []*workloadmeta.KubernetesResourceClaim
	slices []*workloadmeta.KubernetesResourceSlice
	pods   []*workloadmeta.KubernetesPod
}

func (f *fakeStore) ListKubernetesResourceClaims() []*workloadmeta.KubernetesResourceClaim {
	return f.claims
}

func (f *fakeStore) ListKubernetesResourceSlices() []*workloadmeta.KubernetesResourceSlice {
	return f.slices
}

func (f *fakeStore) ListKubernetesPods() []*workloadmeta.KubernetesPod {
	return f.pods
}

var now = time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)

// gpuClasses mirrors the shipped default allowlist.
var gpuClasses = map[string]struct{}{
	"gpu.nvidia.com":      {},
	"mig.nvidia.com":      {},
	"vfio.gpu.nvidia.com": {},
}

func TestDRABackendPending(t *testing.T) {
	store := &fakeStore{claims: []*workloadmeta.KubernetesResourceClaim{
		{
			EntityMeta:             workloadmeta.EntityMeta{Name: "waiting", Namespace: "team-a"},
			State:                  workloadmeta.ResourceClaimPending,
			CreationTimestamp:      now.Add(-90 * time.Second),
			RequestedDeviceClasses: []string{"gpu.nvidia.com"},
			OwnerPod:               "trainer-0",
		},
		{
			// Allocated claims are no longer waiting.
			EntityMeta:             workloadmeta.EntityMeta{Name: "running", Namespace: "team-a"},
			State:                  workloadmeta.ResourceClaimReserved,
			NodeName:               "node-1",
			CreationTimestamp:      now.Add(-1 * time.Hour),
			RequestedDeviceClasses: []string{"gpu.nvidia.com"},
			OwnerPod:               "trainer-1",
		},
		{
			// A claim with no creation timestamp reports an unknown (zero)
			// wait rather than the time since the epoch.
			EntityMeta:             workloadmeta.EntityMeta{Name: "no-ts", Namespace: "team-b"},
			State:                  workloadmeta.ResourceClaimPending,
			RequestedDeviceClasses: []string{"mig.nvidia.com"},
			OwnerPod:               "trainer-2",
		},
		{
			// A network-device claim is not an accelerator claim.
			EntityMeta:             workloadmeta.EntityMeta{Name: "nic", Namespace: "team-b"},
			State:                  workloadmeta.ResourceClaimPending,
			CreationTimestamp:      now.Add(-time.Hour),
			RequestedDeviceClasses: []string{"nic.example.com"},
			OwnerPod:               "trainer-3",
		},
		{
			// A pre-created claim with nothing consuming it is not a queued
			// workload.
			EntityMeta:             workloadmeta.EntityMeta{Name: "orphan", Namespace: "team-b"},
			State:                  workloadmeta.ResourceClaimPending,
			CreationTimestamp:      now.Add(-24 * time.Hour),
			RequestedDeviceClasses: []string{"gpu.nvidia.com"},
		},
	}}

	b := &draBackend{store: store, acceleratorClasses: gpuClasses}
	assert.ElementsMatch(t, []pendingAllocation{
		{namespace: "team-a", waiting: 90 * time.Second},
		{namespace: "team-b", waiting: 0},
	}, b.pending(now))
}

func TestDRABackendPendingDeduplicatesWorkloads(t *testing.T) {
	now := time.Now()
	store := &fakeStore{claims: []*workloadmeta.KubernetesResourceClaim{
		{
			// Two generated claims for the same pod are one workload waiting,
			// not two -- the longest wait is kept.
			EntityMeta:             workloadmeta.EntityMeta{Name: "c1", Namespace: "team-a"},
			State:                  workloadmeta.ResourceClaimPending,
			CreationTimestamp:      now.Add(-90 * time.Second),
			RequestedDeviceClasses: []string{"gpu.nvidia.com"},
			OwnerPod:               "trainer-0",
		},
		{
			EntityMeta:             workloadmeta.EntityMeta{Name: "c2", Namespace: "team-a"},
			State:                  workloadmeta.ResourceClaimPending,
			CreationTimestamp:      now.Add(-30 * time.Second),
			RequestedDeviceClasses: []string{"gpu.nvidia.com"},
			OwnerPod:               "trainer-0",
		},
		{
			// A different pod in the same namespace is a second workload.
			EntityMeta:             workloadmeta.EntityMeta{Name: "c3", Namespace: "team-a"},
			State:                  workloadmeta.ResourceClaimPending,
			CreationTimestamp:      now.Add(-60 * time.Second),
			RequestedDeviceClasses: []string{"gpu.nvidia.com"},
			OwnerPod:               "trainer-1",
		},
		{
			// An administrative claim is for inspection, not allocation -- it is
			// not a workload waiting for devices.
			EntityMeta:             workloadmeta.EntityMeta{Name: "admin", Namespace: "team-a"},
			State:                  workloadmeta.ResourceClaimPending,
			CreationTimestamp:      now.Add(-120 * time.Second),
			RequestedDeviceClasses: []string{"gpu.nvidia.com"},
			OwnerPod:               "admin-pod",
			AdminAccess:            true,
		},
	}}

	b := &draBackend{store: store, acceleratorClasses: gpuClasses}

	assert.ElementsMatch(t, []pendingAllocation{
		// trainer-0's two claims collapse to one, keeping the longest wait.
		{namespace: "team-a", waiting: 90 * time.Second},
		{namespace: "team-a", waiting: 60 * time.Second},
	}, b.pending(now))
}

func TestDRABackendPendingCountsPreCreatedClaimsReferencedByPods(t *testing.T) {
	now := time.Now()
	store := &fakeStore{
		claims: []*workloadmeta.KubernetesResourceClaim{
			{
				// A pre-created claim with no ownerReference, referenced by two pods
				// via spec.resourceClaims[].resourceClaimName. It is a workload
				// waiting for devices, not an orphan -- and each referencing pod is
				// a distinct waiting workload.
				EntityMeta:             workloadmeta.EntityMeta{Name: "shared-claim", Namespace: "team-a"},
				State:                  workloadmeta.ResourceClaimPending,
				CreationTimestamp:      now.Add(-24 * time.Hour), // old claim
				RequestedDeviceClasses: []string{"gpu.nvidia.com"},
			},
			{
				// A genuinely orphaned claim (no owner, no referencing pod) is not
				// a workload waiting for devices.
				EntityMeta:             workloadmeta.EntityMeta{Name: "orphan", Namespace: "team-a"},
				State:                  workloadmeta.ResourceClaimPending,
				CreationTimestamp:      now.Add(-120 * time.Second),
				RequestedDeviceClasses: []string{"gpu.nvidia.com"},
			},
		},
		pods: []*workloadmeta.KubernetesPod{
			{
				EntityMeta:         workloadmeta.EntityMeta{Name: "trainer-0", Namespace: "team-a"},
				ResourceClaimNames: []string{"shared-claim"},
				// The wait starts when the pod was created, not when the (much
				// older) claim was -- otherwise a day-old claim reports a day-long
				// wait for a minute-old pod.
				CreationTimestamp: now.Add(-60 * time.Second),
			},
			{
				EntityMeta:         workloadmeta.EntityMeta{Name: "trainer-1", Namespace: "team-a"},
				ResourceClaimNames: []string{"shared-claim"},
				CreationTimestamp:  now.Add(-30 * time.Second),
			},
		},
	}

	b := &draBackend{store: store, acceleratorClasses: gpuClasses}

	assert.ElementsMatch(t, []pendingAllocation{
		// Two pods share the claim: two waiting workloads, each waiting since its
		// own creation, not since the claim's.
		{namespace: "team-a", waiting: 60 * time.Second},
		{namespace: "team-a", waiting: 30 * time.Second},
	}, b.pending(now))
}

// This store publishes no ResourceSlices, so every device is unresolvable and the
// counts come from the positional fallback in allocated(). That is deliberate: it
// pins the degraded behaviour for clusters where slice collection has not caught
// up. The physical-identity path is covered by the MIG tests below.
func TestDRABackendAllocated(t *testing.T) {
	dev := workloadmeta.ResourceClaimDevice{Name: "gpu-0", Driver: "gpu.nvidia.com"}
	nic := workloadmeta.ResourceClaimDevice{Name: "nic-0", Driver: "nic.example.com"}
	store := &fakeStore{claims: []*workloadmeta.KubernetesResourceClaim{
		{
			EntityMeta: workloadmeta.EntityMeta{Name: "a"},
			State:      workloadmeta.ResourceClaimReserved,
			NodeName:   "node-1",
			Devices:    []workloadmeta.ResourceClaimDevice{dev, dev},
		},
		{
			// Same node: counts add up.
			EntityMeta: workloadmeta.EntityMeta{Name: "b"},
			State:      workloadmeta.ResourceClaimAllocated,
			NodeName:   "node-1",
			Devices:    []workloadmeta.ResourceClaimDevice{dev},
		},
		{
			EntityMeta: workloadmeta.EntityMeta{Name: "c"},
			State:      workloadmeta.ResourceClaimReserved,
			NodeName:   "node-2",
			// The non-accelerator device must not be counted.
			Devices: []workloadmeta.ResourceClaimDevice{dev, nic},
		},
		{
			// Pending claims have no node and must not be counted.
			EntityMeta: workloadmeta.EntityMeta{Name: "d"},
			State:      workloadmeta.ResourceClaimPending,
		},
	}}

	b := &draBackend{store: store, acceleratorClasses: gpuClasses}
	assert.ElementsMatch(t, []allocatedDevices{
		{node: "node-1", count: 3},
		{node: "node-2", count: 1},
	}, b.allocated())
}

// A MIG-capable node carves several MIG instances out of one physical card. The
// claim only records driver-scoped device names ("gpu-0"), so the physical
// identity comes from the ResourceSlice, where MIG devices carry ParentUUID.
// Counting claim devices directly reports 3 accelerators on a node that has 1.
func TestDRABackendAllocatedCollapsesMIGInstancesOntoPhysicalCard(t *testing.T) {
	store := &fakeStore{
		claims: []*workloadmeta.KubernetesResourceClaim{{
			EntityMeta: workloadmeta.EntityMeta{Name: "mig-claim"},
			State:      workloadmeta.ResourceClaimReserved,
			NodeName:   "node-1",
			Devices: []workloadmeta.ResourceClaimDevice{
				{Name: "mig-0", Driver: "gpu.nvidia.com", Pool: "node-1"},
				{Name: "mig-1", Driver: "gpu.nvidia.com", Pool: "node-1"},
				{Name: "mig-2", Driver: "gpu.nvidia.com", Pool: "node-1"},
			},
		}},
		slices: []*workloadmeta.KubernetesResourceSlice{{
			EntityMeta: workloadmeta.EntityMeta{Name: "node-1-gpu"},
			NodeName:   "node-1",
			Driver:     "gpu.nvidia.com",
			Pool:       "node-1",
			Devices: []workloadmeta.ResourceSliceDevice{
				// All three instances are carved from the same H100.
				{Name: "mig-0", ParentUUID: "GPU-aaa", Profile: "1g.10gb"},
				{Name: "mig-1", ParentUUID: "GPU-aaa", Profile: "1g.10gb"},
				{Name: "mig-2", ParentUUID: "GPU-aaa", Profile: "2g.20gb"},
			},
		}},
	}

	b := &draBackend{store: store, acceleratorClasses: gpuClasses}
	assert.ElementsMatch(t, []allocatedDevices{
		{node: "node-1", count: 1},
	}, b.allocated())
}

// Two MIG instances from different physical cards are two accelerators, so the
// collapse must key on the parent card and not simply report 1 whenever MIG is
// in play.
func TestDRABackendAllocatedCountsDistinctPhysicalCards(t *testing.T) {
	store := &fakeStore{
		claims: []*workloadmeta.KubernetesResourceClaim{{
			EntityMeta: workloadmeta.EntityMeta{Name: "mig-claim"},
			State:      workloadmeta.ResourceClaimReserved,
			NodeName:   "node-1",
			Devices: []workloadmeta.ResourceClaimDevice{
				{Name: "mig-0", Driver: "gpu.nvidia.com", Pool: "node-1"},
				{Name: "mig-9", Driver: "gpu.nvidia.com", Pool: "node-1"},
			},
		}},
		slices: []*workloadmeta.KubernetesResourceSlice{{
			EntityMeta: workloadmeta.EntityMeta{Name: "node-1-gpu"},
			NodeName:   "node-1",
			Driver:     "gpu.nvidia.com",
			Pool:       "node-1",
			Devices: []workloadmeta.ResourceSliceDevice{
				{Name: "mig-0", ParentUUID: "GPU-aaa", Profile: "1g.10gb"},
				{Name: "mig-9", ParentUUID: "GPU-bbb", Profile: "1g.10gb"},
			},
		}},
	}

	b := &draBackend{store: store, acceleratorClasses: gpuClasses}
	assert.ElementsMatch(t, []allocatedDevices{
		{node: "node-1", count: 2},
	}, b.allocated())
}

// Whole-card devices are identified by their own UUID, and two claims holding
// the same physical card must not double-count it.
func TestDRABackendAllocatedDeduplicatesWholeCardsAcrossClaims(t *testing.T) {
	store := &fakeStore{
		claims: []*workloadmeta.KubernetesResourceClaim{
			{
				EntityMeta: workloadmeta.EntityMeta{Name: "a"},
				State:      workloadmeta.ResourceClaimReserved,
				NodeName:   "node-1",
				Devices:    []workloadmeta.ResourceClaimDevice{{Name: "gpu-0", Driver: "gpu.nvidia.com", Pool: "node-1"}},
			},
			{
				EntityMeta: workloadmeta.EntityMeta{Name: "b"},
				State:      workloadmeta.ResourceClaimReserved,
				NodeName:   "node-1",
				Devices:    []workloadmeta.ResourceClaimDevice{{Name: "gpu-0", Driver: "gpu.nvidia.com", Pool: "node-1"}},
			},
		},
		slices: []*workloadmeta.KubernetesResourceSlice{{
			EntityMeta: workloadmeta.EntityMeta{Name: "node-1-gpu"},
			NodeName:   "node-1",
			Driver:     "gpu.nvidia.com",
			Pool:       "node-1",
			Devices:    []workloadmeta.ResourceSliceDevice{{Name: "gpu-0", UUID: "GPU-aaa"}},
		}},
	}

	b := &draBackend{store: store, acceleratorClasses: gpuClasses}
	assert.ElementsMatch(t, []allocatedDevices{
		{node: "node-1", count: 1},
	}, b.allocated())
}

// Slices may be absent: collection can be lagging, or the driver may publish
// none. Falling back to counting claim devices keeps the metric populated
// instead of silently reporting zero accelerators on a busy node.
func TestDRABackendAllocatedFallsBackWhenSliceIsMissing(t *testing.T) {
	store := &fakeStore{
		claims: []*workloadmeta.KubernetesResourceClaim{{
			EntityMeta: workloadmeta.EntityMeta{Name: "a"},
			State:      workloadmeta.ResourceClaimReserved,
			NodeName:   "node-1",
			Devices: []workloadmeta.ResourceClaimDevice{
				{Name: "gpu-0", Driver: "gpu.nvidia.com", Pool: "node-1"},
				{Name: "gpu-1", Driver: "gpu.nvidia.com", Pool: "node-1"},
			},
		}},
		// No slices collected at all.
	}

	b := &draBackend{store: store, acceleratorClasses: gpuClasses}
	assert.ElementsMatch(t, []allocatedDevices{
		{node: "node-1", count: 2},
	}, b.allocated())
}

// A driver's name and its DeviceClass name need not match. DraNet is a real
// example: DeviceClass "dranet", driver "dra.net". Classifying an allocated
// device by its driver therefore misses it, even though the claim asked for a
// configured class. The device's request names the class, so use that.
func TestDRABackendAllocatedClassifiesByRequestedClassNotDriverName(t *testing.T) {
	store := &fakeStore{claims: []*workloadmeta.KubernetesResourceClaim{{
		EntityMeta:             workloadmeta.EntityMeta{Name: "nic-claim", Namespace: "team-a"},
		State:                  workloadmeta.ResourceClaimReserved,
		NodeName:               "node-1",
		RequestedDeviceClasses: []string{"dranet"},
		DeviceClassByRequest:   map[string]string{"nic": "dranet"},
		Devices: []workloadmeta.ResourceClaimDevice{
			{Name: "ddnet0", Driver: "dra.net", Pool: "node-1", Request: "nic"},
		},
	}}}

	// Configured with the DeviceClass name, which is what an operator reads off
	// `kubectl get deviceclasses`.
	classes := map[string]struct{}{"dranet": {}}
	b := &draBackend{store: store, acceleratorClasses: classes}
	assert.ElementsMatch(t, []allocatedDevices{
		{node: "node-1", count: 1},
	}, b.allocated())
}

// The same mechanism must still exclude: a claim whose request names a class
// that is not configured contributes nothing, even though nothing about the
// driver name says so.
func TestDRABackendAllocatedExcludesUnconfiguredRequestedClass(t *testing.T) {
	store := &fakeStore{claims: []*workloadmeta.KubernetesResourceClaim{{
		EntityMeta:             workloadmeta.EntityMeta{Name: "nic-claim", Namespace: "team-a"},
		State:                  workloadmeta.ResourceClaimReserved,
		NodeName:               "node-1",
		RequestedDeviceClasses: []string{"dranet"},
		DeviceClassByRequest:   map[string]string{"nic": "dranet"},
		Devices: []workloadmeta.ResourceClaimDevice{
			{Name: "ddnet0", Driver: "dra.net", Pool: "node-1", Request: "nic"},
		},
	}}}

	b := &draBackend{store: store, acceleratorClasses: gpuClasses}
	assert.Empty(t, b.allocated())
}

// A claim may mix accelerator and non-accelerator requests. Only the devices
// whose own request names a configured class count, which is precisely what
// classifying per device rather than per claim buys.
func TestDRABackendAllocatedCountsOnlyAcceleratorRequestsInMixedClaim(t *testing.T) {
	store := &fakeStore{claims: []*workloadmeta.KubernetesResourceClaim{{
		EntityMeta:             workloadmeta.EntityMeta{Name: "mixed", Namespace: "team-a"},
		State:                  workloadmeta.ResourceClaimReserved,
		NodeName:               "node-1",
		RequestedDeviceClasses: []string{"gpu.nvidia.com", "dranet"},
		DeviceClassByRequest: map[string]string{
			"accel": "gpu.nvidia.com",
			"nic":   "dranet",
		},
		Devices: []workloadmeta.ResourceClaimDevice{
			{Name: "gpu-0", Driver: "gpu.nvidia.com", Pool: "node-1", Request: "accel"},
			{Name: "ddnet0", Driver: "dra.net", Pool: "node-1", Request: "nic"},
		},
	}}}

	b := &draBackend{store: store, acceleratorClasses: gpuClasses}
	assert.ElementsMatch(t, []allocatedDevices{
		{node: "node-1", count: 1},
	}, b.allocated())
}

// The supply side must be indexed by the same rule the demand side uses. A
// vendor whose driver name differs from its DeviceClass name has its slices
// skipped when the index filters on driver, so its MIG instances never collapse
// onto a parent card and fall back to positional counting instead.
func TestDRABackendAllocatedDeduplicatesForVendorWhoseDriverNameDiffersFromClass(t *testing.T) {
	store := &fakeStore{
		claims: []*workloadmeta.KubernetesResourceClaim{{
			EntityMeta:             workloadmeta.EntityMeta{Name: "accel", Namespace: "team-a"},
			State:                  workloadmeta.ResourceClaimReserved,
			NodeName:               "node-1",
			RequestedDeviceClasses: []string{"accel.example.com"},
			DeviceClassByRequest:   map[string]string{"a": "accel.example.com"},
			Devices: []workloadmeta.ResourceClaimDevice{
				// Three partitions of ONE physical card.
				{Name: "part-0", Driver: "vendor.example.io", Pool: "node-1", Request: "a"},
				{Name: "part-1", Driver: "vendor.example.io", Pool: "node-1", Request: "a"},
				{Name: "part-2", Driver: "vendor.example.io", Pool: "node-1", Request: "a"},
			},
		}},
		slices: []*workloadmeta.KubernetesResourceSlice{{
			EntityMeta: workloadmeta.EntityMeta{Name: "node-1-accel"},
			NodeName:   "node-1",
			// Driver name deliberately unequal to the DeviceClass name.
			Driver: "vendor.example.io",
			Pool:   "node-1",
			Devices: []workloadmeta.ResourceSliceDevice{
				{Name: "part-0", ParentUUID: "ACCEL-aaa"},
				{Name: "part-1", ParentUUID: "ACCEL-aaa"},
				{Name: "part-2", ParentUUID: "ACCEL-aaa"},
			},
		}},
	}

	b := &draBackend{store: store, acceleratorClasses: map[string]struct{}{"accel.example.com": {}}}
	assert.ElementsMatch(t, []allocatedDevices{
		{node: "node-1", count: 1},
	}, b.allocated())
}

// Kubernetes requires consumers to use only the highest pool generation: a
// driver bumps the generation on every slice in the pool when anything changes,
// so a pool caught mid-update has both old and new slices present. Indexing all
// of them mixes stale inventory with current, which can map a device name onto a
// parent card it no longer belongs to.
func TestDRABackendAllocatedIgnoresStalePoolGenerations(t *testing.T) {
	store := &fakeStore{
		claims: []*workloadmeta.KubernetesResourceClaim{{
			EntityMeta:             workloadmeta.EntityMeta{Name: "c", Namespace: "team-a"},
			State:                  workloadmeta.ResourceClaimReserved,
			NodeName:               "node-1",
			RequestedDeviceClasses: []string{"gpu.nvidia.com"},
			DeviceClassByRequest:   map[string]string{"a": "gpu.nvidia.com"},
			Devices: []workloadmeta.ResourceClaimDevice{
				{Name: "mig-0", Driver: "gpu.nvidia.com", Pool: "node-1", Request: "a"},
				{Name: "mig-1", Driver: "gpu.nvidia.com", Pool: "node-1", Request: "a"},
			},
		}},
		// Deliberately listed newest-first: the store gives no ordering guarantee,
		// so correctness must come from comparing generations, not from the stale
		// entry happening to be overwritten by a later one.
		slices: []*workloadmeta.KubernetesResourceSlice{
			{
				// Current: the driver re-partitioned, both are on one card now.
				EntityMeta:     workloadmeta.EntityMeta{Name: "new"},
				NodeName:       "node-1",
				Driver:         "gpu.nvidia.com",
				Pool:           "node-1",
				PoolGeneration: 2,
				Devices: []workloadmeta.ResourceSliceDevice{
					{Name: "mig-0", ParentUUID: "GPU-ccc"},
					{Name: "mig-1", ParentUUID: "GPU-ccc"},
				},
			},
			{
				// Stale: at generation 1 these two devices sat on different cards.
				EntityMeta:     workloadmeta.EntityMeta{Name: "old"},
				NodeName:       "node-1",
				Driver:         "gpu.nvidia.com",
				Pool:           "node-1",
				PoolGeneration: 1,
				Devices: []workloadmeta.ResourceSliceDevice{
					{Name: "mig-0", ParentUUID: "GPU-aaa"},
					{Name: "mig-1", ParentUUID: "GPU-bbb"},
				},
			},
		},
	}

	b := &draBackend{store: store, acceleratorClasses: gpuClasses}
	// Only generation 2 counts: one physical card, not two.
	assert.ElementsMatch(t, []allocatedDevices{
		{node: "node-1", count: 1},
	}, b.allocated())
}

// A slice at the newest generation is authoritative about the devices it
// itself lists, regardless of whether a sibling slice of the same pool has
// caught up yet. Kubernetes' own doc reserves the ResourceSliceCount
// completeness check for a different job -- "allocating all resources in a
// pool" or "looking for the best solution among several alternatives" -- not
// for resolving one already-allocated device's identity. Gating the identity
// lookup on pool completeness was tried and was wrong: it disqualifies the
// entire pool the moment any sibling slice lags, which is exactly the
// multi-slice-pool case (ResourceSliceMaxDevices=128 forces MIG-heavy pools to
// split) this feature's MIG collapse targets most.
func TestDRABackendAllocatedTrustsNewestGenerationEvenWhenSiblingSliceLags(t *testing.T) {
	store := &fakeStore{
		claims: []*workloadmeta.KubernetesResourceClaim{{
			EntityMeta: workloadmeta.EntityMeta{Name: "mig-claim"},
			State:      workloadmeta.ResourceClaimReserved,
			NodeName:   "node-1",
			Devices: []workloadmeta.ResourceClaimDevice{
				// Two MIG instances of one card, described by the slice that
				// HAS reached the new generation.
				{Name: "mig-0", Driver: "gpu.nvidia.com", Pool: "node-1"},
				{Name: "mig-1", Driver: "gpu.nvidia.com", Pool: "node-1"},
				// A second, whole card described by the slice that has NOT
				// caught up to the new generation yet.
				{Name: "gpu-4", Driver: "gpu.nvidia.com", Pool: "node-1"},
			},
		}},
		slices: []*workloadmeta.KubernetesResourceSlice{
			{
				// Reached generation 6: this card's MIG layout is current.
				EntityMeta:     workloadmeta.EntityMeta{Name: "slice-a"},
				NodeName:       "node-1",
				Driver:         "gpu.nvidia.com",
				Pool:           "node-1",
				PoolGeneration: 6,
				Devices: []workloadmeta.ResourceSliceDevice{
					{Name: "mig-0", ParentUUID: "GPU-aaa"},
					{Name: "mig-1", ParentUUID: "GPU-aaa"},
				},
			},
			{
				// Still at generation 5 -- the driver has not republished it
				// under 6 yet, even though its own device list is unchanged.
				EntityMeta:     workloadmeta.EntityMeta{Name: "slice-b"},
				NodeName:       "node-1",
				Driver:         "gpu.nvidia.com",
				Pool:           "node-1",
				PoolGeneration: 5,
				Devices: []workloadmeta.ResourceSliceDevice{
					{Name: "gpu-4", UUID: "GPU-bbb"},
				},
			},
		},
	}

	b := &draBackend{store: store, acceleratorClasses: gpuClasses}
	// Ground truth is 2 physical accelerators. slice-a@6 is trusted on its own
	// merits and collapses the two MIG instances to 1; gpu-4's only known slice
	// is at the superseded generation 5, so it is not in the index and is
	// counted positionally as the other 1. A completeness gate would instead
	// disqualify the whole pool and report 3 (all devices unresolved) --
	// verified by this test failing against that implementation.
	assert.ElementsMatch(t, []allocatedDevices{
		{node: "node-1", count: 2},
	}, b.allocated())
}

// adminAccess devices are monitoring/management access to a card, not workload
// consumption of it. Counting them inflates devices.allocated: an admin tool
// watching a GPU would make that GPU look allocated to a workload.
func TestDRABackendAllocatedExcludesAdminAccessDevices(t *testing.T) {
	store := &fakeStore{
		claims: []*workloadmeta.KubernetesResourceClaim{{
			EntityMeta:             workloadmeta.EntityMeta{Name: "mixed", Namespace: "team-a"},
			State:                  workloadmeta.ResourceClaimReserved,
			NodeName:               "node-1",
			RequestedDeviceClasses: []string{"gpu.nvidia.com"},
			DeviceClassByRequest:   map[string]string{"a": "gpu.nvidia.com", "admin": "gpu.nvidia.com"},
			Devices: []workloadmeta.ResourceClaimDevice{
				{Name: "gpu-0", Driver: "gpu.nvidia.com", Pool: "node-1", Request: "a"},
				// Admin access to a *different* physical card: without the
				// exclusion this would add a second accelerator to the count.
				{Name: "gpu-1", Driver: "gpu.nvidia.com", Pool: "node-1", Request: "admin", AdminAccess: true},
			},
		}},
		slices: []*workloadmeta.KubernetesResourceSlice{{
			EntityMeta: workloadmeta.EntityMeta{Name: "node-1-gpu"},
			NodeName:   "node-1",
			Driver:     "gpu.nvidia.com",
			Pool:       "node-1",
			Devices: []workloadmeta.ResourceSliceDevice{
				{Name: "gpu-0", UUID: "GPU-aaa"},
				{Name: "gpu-1", UUID: "GPU-bbb"},
			},
		}},
	}

	b := &draBackend{store: store, acceleratorClasses: gpuClasses}
	assert.ElementsMatch(t, []allocatedDevices{
		{node: "node-1", count: 1},
	}, b.allocated())
}

func TestWaitingSinceClampsFutureAndZero(t *testing.T) {
	assert.Equal(t, time.Duration(0), waitingSince(now, time.Time{}))
	assert.Equal(t, time.Duration(0), waitingSince(now, now.Add(time.Minute)))
	assert.Equal(t, time.Minute, waitingSince(now, now.Add(-time.Minute)))
}
