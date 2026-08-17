// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package kubernetesresourceparsers

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

type resourceClaimParser struct{}

// NewResourceClaimParser returns a parser for DRA ResourceClaim resources.
func NewResourceClaimParser() ObjectParser {
	return resourceClaimParser{}
}

func (p resourceClaimParser) Parse(obj interface{}) workloadmeta.Entity {
	u := obj.(*unstructured.Unstructured)
	meta := workloadmeta.EntityMeta{
		Name:        u.GetName(),
		Namespace:   u.GetNamespace(),
		Labels:      u.GetLabels(),
		Annotations: u.GetAnnotations(),
		UID:         string(u.GetUID()),
	}

	claim := &workloadmeta.KubernetesResourceClaim{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesResourceClaim,
			ID:   workloadmeta.GenerateResourceClaimEntityID(meta.Namespace, meta.Name),
		},
		EntityMeta:        meta,
		OwnerPod:          ownerPodName(u),
		Devices:           allocatedDevices(u),
		ReservedForPods:   reservedForPods(u),
		CreationTimestamp: u.GetCreationTimestamp().Time,
	}
	claim.RequestedDeviceClasses, claim.DeviceClassByRequest = requestedDeviceClasses(u)
	claim.AdminAccess = hasAdminAccess(u)
	claim.NodeName = allocationNodeName(u)
	claim.State = deriveState(len(claim.Devices) > 0, len(claim.ReservedForPods) > 0)

	return claim
}

// deriveState maps the DRA status onto the Job Timeline state machine:
// no allocation -> pending (waiting for GPUs); allocated but not reserved ->
// allocated; allocated and reserved -> reserved (running on its accelerators).
func deriveState(allocated, reserved bool) workloadmeta.ResourceClaimState {
	switch {
	case allocated && reserved:
		return workloadmeta.ResourceClaimReserved
	case allocated:
		return workloadmeta.ResourceClaimAllocated
	default:
		return workloadmeta.ResourceClaimPending
	}
}

func ownerPodName(u *unstructured.Unstructured) string {
	for _, ref := range u.GetOwnerReferences() {
		if ref.Kind == kubernetes.PodKind {
			return ref.Name
		}
	}
	return ""
}

// hasAdminAccess reports whether any request is marked administrative. An
// administrative claim is created for inspection, not allocation, so it must not
// be counted as a waiting workload.
//
// The flag lives in two places depending on API version: v1 nests it inside the
// "exactly" wrapper (ExactDeviceRequest.adminAccess); v1beta1 puts it directly
// on the request. The GVR pins no version, so both paths must be read.
func hasAdminAccess(u *unstructured.Unstructured) bool {
	requests, found, _ := unstructured.NestedSlice(u.Object, "spec", "devices", "requests")
	if !found {
		return false
	}
	for _, request := range requests {
		requestMap, ok := request.(map[string]interface{})
		if !ok {
			continue
		}
		// v1: inside the "exactly" wrapper.
		if admin, _, _ := unstructured.NestedBool(requestMap, "exactly", "adminAccess"); admin {
			return true
		}
		// v1beta1: directly on the request.
		if admin, _, _ := unstructured.NestedBool(requestMap, "adminAccess"); admin {
			return true
		}
	}
	return false
}

// requestedDeviceClasses collects the device classes a claim asks for. A request
// either names one class outright ("exactly") or offers alternatives
// ("firstAvailable"); both shapes are read so the claim can be classified before
// the scheduler has allocated anything.
func requestedDeviceClasses(u *unstructured.Unstructured) ([]string, map[string]string) {
	requests, found, _ := unstructured.NestedSlice(u.Object, "spec", "devices", "requests")
	if !found {
		return nil, nil
	}

	seen := make(map[string]struct{})
	var classes []string
	byRequest := make(map[string]string)
	add := func(class string) {
		if class == "" {
			return
		}
		if _, dup := seen[class]; dup {
			return
		}
		seen[class] = struct{}{}
		classes = append(classes, class)
	}

	for _, request := range requests {
		requestMap, ok := request.(map[string]interface{})
		if !ok {
			continue
		}
		name, _, _ := unstructured.NestedString(requestMap, "name")

		// v1/v1beta2 wrap a single request in "exactly"; v1beta1 puts
		// deviceClassName directly on the request. The GVR pins no version --
		// it is resolved by discovery -- so both shapes must be read or a
		// v1beta1 cluster silently yields no classes at all.
		class, _, _ := unstructured.NestedString(requestMap, "exactly", "deviceClassName")
		if class == "" {
			class, _, _ = unstructured.NestedString(requestMap, "deviceClassName")
		}
		if class != "" {
			add(class)
			if name != "" {
				byRequest[name] = class
			}
			continue
		}

		alternatives, ok, _ := unstructured.NestedSlice(requestMap, "firstAvailable")
		if !ok {
			continue
		}
		for _, alternative := range alternatives {
			alternativeMap, ok := alternative.(map[string]interface{})
			if !ok {
				continue
			}
			class, _, _ := unstructured.NestedString(alternativeMap, "deviceClassName")
			add(class)

			// An allocation result names a subrequest as "<request>/<subrequest>",
			// so key it that way or the device will not resolve back to its class.
			subName, _, _ := unstructured.NestedString(alternativeMap, "name")
			if name != "" && subName != "" && class != "" {
				byRequest[name+"/"+subName] = class
			}
		}
	}

	if len(byRequest) == 0 {
		byRequest = nil
	}
	return classes, byRequest
}

func allocatedDevices(u *unstructured.Unstructured) []workloadmeta.ResourceClaimDevice {
	results, found, _ := unstructured.NestedSlice(u.Object, "status", "allocation", "devices", "results")
	if !found {
		return nil
	}

	devices := make([]workloadmeta.ResourceClaimDevice, 0, len(results))
	for _, result := range results {
		resultMap, ok := result.(map[string]interface{})
		if !ok {
			continue
		}
		name, _, _ := unstructured.NestedString(resultMap, "device")
		// A result with no device name cannot be joined to a ResourceSlice (the
		// join key is (driver, pool, name)); skipping it matches the ResourceSlice
		// parser's defensive behaviour and avoids inflating devices.allocated with
		// malformed or partial results.
		if name == "" {
			continue
		}
		driver, _, _ := unstructured.NestedString(resultMap, "driver")
		pool, _, _ := unstructured.NestedString(resultMap, "pool")
		request, _, _ := unstructured.NestedString(resultMap, "request")
		// Absent or false both mean ordinary consumption; only an explicit true
		// marks administrative access.
		adminAccess, _, _ := unstructured.NestedBool(resultMap, "adminAccess")
		devices = append(devices, workloadmeta.ResourceClaimDevice{
			Name:        name,
			Driver:      driver,
			Pool:        pool,
			Request:     request,
			AdminAccess: adminAccess,
		})
	}
	return devices
}

func reservedForPods(u *unstructured.Unstructured) []string {
	reserved, found, _ := unstructured.NestedSlice(u.Object, "status", "reservedFor")
	if !found {
		return nil
	}

	pods := make([]string, 0, len(reserved))
	for _, ref := range reserved {
		refMap, ok := ref.(map[string]interface{})
		if !ok {
			continue
		}
		resource, _, _ := unstructured.NestedString(refMap, "resource")
		if resource != "pods" {
			continue
		}
		if name, _, _ := unstructured.NestedString(refMap, "name"); name != "" {
			pods = append(pods, name)
		}
	}
	return pods
}

// allocationNodeName extracts the node the devices were allocated on from the
// allocation nodeSelector. A node-local device is encoded as a single
// matchFields term keyed on metadata.name.
//
// A driver may instead select nodes by label (matchExpressions) for
// non-node-local devices, e.g. network-attached accelerators or a pool spread
// across several nodes -- the doc on ResourcePool notes the node name "is not
// required". That shape is not handled: it has no single owning node to tag
// devices.allocated by, and no such claim has been observed on the internal
// staging cluster (venomoth: 51/51 claims are node-local). Such a claim
// returns "" here and its devices are silently excluded by allocated(), which
// is worth knowing about if this metric ever looks low on a cluster running
// such a driver -- see the debug log there.
func allocationNodeName(u *unstructured.Unstructured) string {
	terms, found, _ := unstructured.NestedSlice(u.Object, "status", "allocation", "nodeSelector", "nodeSelectorTerms")
	if !found {
		return ""
	}
	for _, term := range terms {
		termMap, ok := term.(map[string]interface{})
		if !ok {
			continue
		}
		exprs, ok, _ := unstructured.NestedSlice(termMap, "matchFields")
		if !ok {
			continue
		}
		for _, expr := range exprs {
			exprMap, ok := expr.(map[string]interface{})
			if !ok {
				continue
			}
			field, _, _ := unstructured.NestedString(exprMap, "key")
			if field != "metadata.name" {
				continue
			}
			values, _, _ := unstructured.NestedStringSlice(exprMap, "values")
			if len(values) > 0 {
				return values[0]
			}
		}
	}
	return ""
}
