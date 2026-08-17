// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package gpuallocation

import (
	"time"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// allocationSource identifies which mechanism assigned accelerators to a
// workload. The questions this check answers are the same either way, but the
// data comes from different objects, so the source is carried as a tag.
const (
	sourceDRA = "dra"
)

// pendingAllocation is a workload that has asked for accelerators and does not
// have them yet: its ResourceClaim exists but the scheduler has made no
// allocation.
type pendingAllocation struct {
	namespace string
	// waiting is how long the workload has been waiting so far.
	waiting time.Duration
}

// allocatedDevices is a count of accelerators handed to workloads on one node.
type allocatedDevices struct {
	node  string
	count int
}

// backend reads one allocation mechanism out of workloadmeta. Only DRA is
// implemented: the legacy device-plugin path is pod-scheduling latency, which is
// a different problem owned elsewhere (AGTINFR-1184), not accelerator allocation.
//
// The interface is kept so a second accelerator-allocation source can be added
// without reshaping the check.
type backend interface {
	// name is the value used for the `source` tag.
	name() string
	// pending returns the workloads still waiting for accelerators.
	pending(now time.Time) []pendingAllocation
	// allocated returns per-node device counts, or nil when the backend
	// cannot determine them.
	allocated() []allocatedDevices
}

// draBackend reads DRA ResourceClaim entities. It is the richer of the two: the
// claim records the devices themselves, not just a count.
type draBackend struct {
	store workloadmeta.Component
	// acceleratorClasses restricts which device classes count. ResourceClaims
	// are generic and may request network devices or FPGAs, so without this
	// every claim in the cluster would be reported as an accelerator claim.
	acceleratorClasses map[string]struct{}
}

// isAcceleratorClaim reports whether a claim asks for one of the configured
// accelerator device classes.
func isAcceleratorClaim(claim *workloadmeta.KubernetesResourceClaim, acceleratorClasses map[string]struct{}) bool {
	for _, class := range claim.RequestedDeviceClasses {
		if _, ok := acceleratorClasses[class]; ok {
			return true
		}
	}
	return false
}

// isAcceleratorDevice reports whether one allocated device is an accelerator.
//
// The device's request names the DeviceClass it was allocated against, so that
// is the authoritative answer and is what is used when available. Matching on
// the driver name instead only works when a driver's name equals its class
// name: true for NVIDIA ("gpu.nvidia.com" is both), false in general. DraNet
// publishes DeviceClass "dranet" from driver "dra.net", and was verified to be
// missed entirely by driver matching.
//
// The driver check remains as a fallback for entities collected before the
// per-request mapping existed, where it is still right for NVIDIA.
func (b *draBackend) isAcceleratorDevice(claim *workloadmeta.KubernetesResourceClaim, device workloadmeta.ResourceClaimDevice) bool {
	if class, ok := claim.DeviceClassByRequest[device.Request]; ok {
		_, isAccelerator := b.acceleratorClasses[class]
		return isAccelerator
	}
	return b.isAcceleratorDriver(device.Driver)
}

// isAcceleratorDriver reports whether a device came from a driver whose name is
// a configured accelerator class. Only sound where driver and class names
// coincide -- see isAcceleratorDevice, which should be preferred.
func (b *draBackend) isAcceleratorDriver(driver string) bool {
	_, ok := b.acceleratorClasses[driver]
	return ok
}

func (b *draBackend) name() string { return sourceDRA }

func (b *draBackend) pending(now time.Time) []pendingAllocation {
	// A claim referenced by a pod via spec.resourceClaims[].resourceClaimName
	// carries no ownerReference, but is still a workload waiting for devices.
	// Build a (namespace, claimName) -> referencing pods map so such claims are
	// not discarded as orphans. A claim can be shared by several pods, so the
	// map holds a set, and each consumer is a distinct waiting workload.
	claimToPods := make(map[string]map[string]time.Time)
	for _, pod := range b.store.ListKubernetesPods() {
		for _, claimName := range pod.ResourceClaimNames {
			key := pod.Namespace + "/" + claimName
			if claimToPods[key] == nil {
				claimToPods[key] = make(map[string]time.Time)
			}
			claimToPods[key][pod.Name] = pod.CreationTimestamp
		}
	}

	// One entry per workload, not per claim. A pod can hold several generated
	// ResourceClaims (DRAExtendedResource, or explicit multi-claim pods); each
	// pending claim would otherwise double-count the same waiting workload.
	// Deduplicate on (namespace, OwnerPod) and keep the longest wait.
	byWorkload := make(map[string]pendingAllocation)
	add := func(namespace, ownerPod string, waitStart time.Time) {
		key := namespace + "/" + ownerPod
		waiting := waitingSince(now, waitStart)
		if existing, ok := byWorkload[key]; !ok || waiting > existing.waiting {
			byWorkload[key] = pendingAllocation{namespace: namespace, waiting: waiting}
		}
	}
	for _, claim := range b.store.ListKubernetesResourceClaims() {
		if claim.State != workloadmeta.ResourceClaimPending {
			continue
		}
		if !isAcceleratorClaim(claim, b.acceleratorClasses) {
			continue
		}
		// Administrative claims are created for inspection, not allocation; they
		// are not workloads waiting for devices.
		if claim.AdminAccess {
			continue
		}
		if claim.OwnerPod != "" {
			// A generated claim is created with its pod, so the claim creation
			// time is the start of the wait.
			add(claim.Namespace, claim.OwnerPod, claim.CreationTimestamp)
			continue
		}
		// A pre-created claim referenced by name: each referencing pod is a
		// distinct waiting workload, and the wait starts when the pod was
		// created, not when the (possibly much older) claim was.
		for podName, podCreated := range claimToPods[claim.Namespace+"/"+claim.Name] {
			add(claim.Namespace, podName, podCreated)
		}
	}
	out := make([]pendingAllocation, 0, len(byWorkload))
	for _, p := range byWorkload {
		out = append(out, p)
	}
	return out
}

// sliceDeviceKey identifies one advertised device. A device name is only unique
// within its driver's pool, so all three parts are needed to join a claim's
// allocation back to the inventory that describes it.
type sliceDeviceKey struct {
	driver string
	pool   string
	name   string
}

// physicalDeviceIndex maps an allocated device name to the physical accelerator
// it belongs to. MIG instances resolve to their parent card, so several
// instances carved from one GPU collapse onto a single identity.
type physicalDeviceIndex map[sliceDeviceKey]string

// buildPhysicalDeviceIndex indexes the supply side (ResourceSlices) so allocated
// device names can be resolved to physical identities.
// poolKey identifies one resource pool. Generations are per-pool, so they are
// only comparable within the same driver and pool name.
type poolKey struct {
	driver string
	pool   string
}

// highestPoolGenerations returns, per pool, the newest generation observed.
//
// Kubernetes' ResourceSlice doc is direct: "A consumer must only use
// ResourceSlices with the highest generation number and ignore all others."
// That rule has no completeness precondition -- a slice at the newest
// generation is authoritative about the devices it itself lists, regardless of
// whether a sibling slice of the same pool has caught up to that generation
// yet. ResourceSliceCount exists for a different job: "when allocating all
// resources in a pool ... or looking for the best solution among several
// alternatives", not for resolving one already-allocated device's identity, so
// it does not gate this index. (An earlier version of this function did gate
// on it, which was wrong: it disqualified an entire multi-slice pool the
// moment any one sibling lagged -- exactly the MIG-heavy case
// ResourceSliceMaxDevices=128 forces into multiple slices, i.e. the case this
// feature's MIG collapse exists for.)
func highestPoolGenerations(slices []*workloadmeta.KubernetesResourceSlice) map[poolKey]int64 {
	newest := make(map[poolKey]int64)
	for _, slice := range slices {
		key := poolKey{driver: slice.Driver, pool: slice.Pool}
		if generation, seen := newest[key]; !seen || slice.PoolGeneration > generation {
			newest[key] = slice.PoolGeneration
		}
	}
	return newest
}

func (b *draBackend) buildPhysicalDeviceIndex() physicalDeviceIndex {
	index := make(physicalDeviceIndex)
	slices := b.store.ListKubernetesResourceSlices()
	newestGeneration := highestPoolGenerations(slices)

	for _, slice := range slices {
		// Skip superseded slices. Without this the result depends on store
		// ordering, which is not guaranteed, so an older slice could overwrite
		// current inventory and map a device onto a card it no longer sits on.
		// A device whose only known slice is at an older generation is simply
		// not in the index yet -- it degrades to positional counting below,
		// the same as a device with no slice at all.
		if slice.PoolGeneration < newestGeneration[poolKey{driver: slice.Driver, pool: slice.Pool}] {
			continue
		}
		// Deliberately not filtered by driver. This index only maps a device
		// name to a physical identity; deciding whether a device counts as an
		// accelerator is the claim's job (isAcceleratorDevice), because that is
		// where the DeviceClass is known. Filtering here on driver name would
		// silently drop every vendor whose driver name differs from its
		// DeviceClass name -- the same defect isAcceleratorDevice exists to
		// avoid -- leaving their MIG instances to be counted positionally.
		//
		// The key includes the driver, so entries from unrelated drivers cannot
		// collide with an accelerator's.
		for _, device := range slice.Devices {
			// A MIG instance carries no uuid of its own; parentUUID is the
			// physical card it was carved from. Preferring it is what stops
			// several instances of one GPU counting as several GPUs.
			identity := device.ParentUUID
			if identity == "" {
				identity = device.UUID
			}
			if identity == "" {
				// Nothing to deduplicate on.
				continue
			}
			index[sliceDeviceKey{driver: slice.Driver, pool: slice.Pool, name: device.Name}] = identity
		}
	}
	return index
}

func (b *draBackend) allocated() []allocatedDevices {
	index := b.buildPhysicalDeviceIndex()

	// Per node, the set of distinct physical accelerators in use. A set rather
	// than a counter because one card can appear under several claims (MIG
	// instances of it, or the same whole card allocated twice).
	perNode := make(map[string]map[string]struct{})
	// Devices whose physical identity is unknown are counted positionally
	// instead, so the metric degrades to the old behaviour rather than to zero.
	unresolvedPerNode := make(map[string]int)

	for _, claim := range b.store.ListKubernetesResourceClaims() {
		if len(claim.Devices) == 0 {
			continue
		}
		// Devices are only meaningful once the scheduler has picked a node.
		// A claim allocated via a label-based nodeSelector (non-node-local
		// devices, e.g. network-attached accelerators) has no single owning
		// node and is excluded here -- see allocationNodeName. Logged because
		// it is otherwise a silent gap in devices.allocated.
		if claim.NodeName == "" {
			log.Debugf("gpu_allocation: claim %s/%s has allocated devices but no NodeName (label-based nodeSelector?); excluded from devices.allocated", claim.Namespace, claim.Name)
			continue
		}
		for _, device := range claim.Devices {
			if !b.isAcceleratorDevice(claim, device) {
				continue
			}
			// Administrative access is monitoring or management of a device,
			// not workload consumption of it -- Kubernetes has such claims
			// ignore ordinary claims to the device entirely. Counting it would
			// make a card being watched look like a card being used.
			if device.AdminAccess {
				continue
			}
			identity, ok := index[sliceDeviceKey{driver: device.Driver, pool: device.Pool, name: device.Name}]
			if !ok {
				// The slice is missing: collection may lag the claim, or the
				// driver may publish no inventory. Fall back to counting the
				// allocation itself.
				unresolvedPerNode[claim.NodeName]++
				continue
			}
			if perNode[claim.NodeName] == nil {
				perNode[claim.NodeName] = make(map[string]struct{})
			}
			perNode[claim.NodeName][identity] = struct{}{}
		}
	}

	out := make([]allocatedDevices, 0, len(perNode))
	for node, physical := range perNode {
		out = append(out, allocatedDevices{node: node, count: len(physical) + unresolvedPerNode[node]})
	}
	// Nodes whose devices were all unresolved have no entry above.
	for node, count := range unresolvedPerNode {
		if _, ok := perNode[node]; ok {
			continue
		}
		out = append(out, allocatedDevices{node: node, count: count})
	}
	return out
}

// waitingSince returns how long ago start was, clamped at zero. A zero start
// means the timestamp was never populated, in which case the wait is unknown
// and reported as zero rather than as the time since the epoch.
func waitingSince(now, start time.Time) time.Duration {
	if start.IsZero() || now.Before(start) {
		return 0
	}
	return now.Sub(start)
}
