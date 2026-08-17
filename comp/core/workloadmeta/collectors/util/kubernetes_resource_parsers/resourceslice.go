// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package kubernetesresourceparsers

import (
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

type resourceSliceParser struct{}

// NewResourceSliceParser returns a parser for DRA ResourceSlice resources.
func NewResourceSliceParser() ObjectParser {
	return resourceSliceParser{}
}

func (p resourceSliceParser) Parse(obj interface{}) workloadmeta.Entity {
	u := obj.(*unstructured.Unstructured)
	meta := workloadmeta.EntityMeta{
		Name:        u.GetName(),
		Namespace:   u.GetNamespace(),
		Labels:      u.GetLabels(),
		Annotations: u.GetAnnotations(),
		UID:         string(u.GetUID()),
	}

	nodeName, _, _ := unstructured.NestedString(u.Object, "spec", "nodeName")
	driver, _, _ := unstructured.NestedString(u.Object, "spec", "driver")
	pool, _, _ := unstructured.NestedString(u.Object, "spec", "pool", "name")
	// Kubernetes requires consumers to consider only the highest generation of a
	// pool, so this is needed to tell current inventory from stale.
	generation, _, _ := unstructured.NestedInt64(u.Object, "spec", "pool", "generation")

	return &workloadmeta.KubernetesResourceSlice{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesResourceSlice,
			ID:   workloadmeta.GenerateResourceSliceEntityID(meta.Name),
		},
		EntityMeta:     meta,
		NodeName:       nodeName,
		Driver:         driver,
		Pool:           pool,
		PoolGeneration: generation,
		Devices:        sliceDevices(u),
	}
}

func sliceDevices(u *unstructured.Unstructured) []workloadmeta.ResourceSliceDevice {
	entries, found, _ := unstructured.NestedSlice(u.Object, "spec", "devices")
	if !found {
		return nil
	}

	devices := make([]workloadmeta.ResourceSliceDevice, 0, len(entries))
	for _, entry := range entries {
		entryMap, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}

		name, _, _ := unstructured.NestedString(entryMap, "name")
		if name == "" {
			continue
		}

		// v1beta1 nests attributes and capacity under "basic"; v1/v1beta2 put
		// them on the device itself. Unwrap "basic" when present.
		fields := entryMap
		if basic, found, _ := unstructured.NestedMap(entryMap, "basic"); found {
			fields = basic
		}

		device := workloadmeta.ResourceSliceDevice{
			Name:        name,
			UUID:        deviceAttributeString(fields, "uuid"),
			ProductName: deviceAttributeString(fields, "productName"),
			PCIeRoot:    deviceAttributeString(fields, "resource.kubernetes.io/pcieRoot"),
			ParentUUID:  deviceAttributeString(fields, "parentUUID"),
			Profile:     deviceAttributeString(fields, "profile"),
			MemoryBytes: deviceCapacityBytes(fields, "memory"),
		}
		devices = append(devices, device)
	}
	return devices
}

// deviceAttributeString reads a DRA device attribute. Attribute values are a
// one-of wrapper, so a string attribute is nested as
// `attributes.<name>.string`; anything not carrying a string reads as empty.
func deviceAttributeString(device map[string]interface{}, name string) string {
	value, _, _ := unstructured.NestedString(device, "attributes", name, "string")
	return value
}

// deviceCapacityBytes reads a DRA device capacity as bytes. Capacities are
// Kubernetes quantities ("81152Mi"), so they need quantity parsing rather than
// a plain integer conversion.
func deviceCapacityBytes(device map[string]interface{}, name string) int64 {
	value, found, _ := unstructured.NestedString(device, "capacity", name, "value")
	if !found || value == "" {
		return 0
	}
	quantity, err := resource.ParseQuantity(value)
	if err != nil {
		return 0
	}
	return quantity.Value()
}
