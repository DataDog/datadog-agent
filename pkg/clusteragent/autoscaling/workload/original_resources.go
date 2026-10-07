// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package workload

import (
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
)

// recordedResourceNames are the resources whose original value is recorded.
var recordedResourceNames = []corev1.ResourceName{corev1.ResourceCPU, corev1.ResourceMemory}

// originalResources is the value of model.OriginalResourcesAnnotation: by container name, the value
// that each cpu and memory request and limit had before the autoscaler first changed it, through the
// webhook at pod creation or an in-place resize. A nil value means the field was absent. A field is
// recorded once: later changes keep the first value, which is the one a recreated pod would get.
type originalResources map[string]*originalContainerResources

type originalContainerResources struct {
	Requests map[corev1.ResourceName]*resource.Quantity `json:"requests,omitempty"`
	Limits   map[corev1.ResourceName]*resource.Quantity `json:"limits,omitempty"`
}

// parseOriginalResources returns the original resources recorded in the pod annotations. ok is false
// when the record is unknown: nothing is then recorded nor reset, rather than taking values set by the
// autoscaler for original ones. The record is unknown when the annotation cannot be parsed, or when it
// is missing on a pod the autoscaler already manages (requireRecordIfManaged): its values may have been
// changed before the record existed (pod admitted or resized by an older Cluster Agent).
func parseOriginalResources(annotations map[string]string, requireRecordIfManaged bool) (original originalResources, ok bool) {
	value, found := annotations[model.OriginalResourcesAnnotation]
	if !found {
		if requireRecordIfManaged && isManagedPod(annotations) {
			return nil, false
		}
		return originalResources{}, true
	}
	if err := json.Unmarshal([]byte(value), &original); err != nil {
		return nil, false
	}
	if original == nil {
		original = originalResources{}
	}
	return original, true
}

// isManagedPod reports whether the autoscaler already manages the pod: the webhook attributed it to an
// autoscaler, or a recommendation was applied to it.
func isManagedPod(annotations map[string]string) bool {
	_, autoscaler := annotations[model.AutoscalerIDAnnotation]
	_, recommendation := annotations[model.RecommendationIDAnnotation]
	return autoscaler || recommendation
}

// merge adds the entries of other that o does not have, and reports whether o changed. Entries of o
// win: it is the record read from the live pod, written first.
func (o originalResources) merge(other originalResources) bool {
	changed := false
	for container, resources := range other {
		for name, value := range resources.Requests {
			changed = o.record(container, false, name, value) || changed
		}
		for name, value := range resources.Limits {
			changed = o.record(container, true, name, value) || changed
		}
	}
	return changed
}

func (o originalResources) encode() (string, error) {
	value, err := json.Marshal(o)
	return string(value), err
}

// record records value (nil: absent) as the original value of a request (limit=false) or limit of
// container, unless one is already recorded. It reports whether the record changed.
func (o originalResources) record(container string, limit bool, name corev1.ResourceName, value *resource.Quantity) bool {
	c := o[container]
	if c == nil {
		c = &originalContainerResources{}
		o[container] = c
	}
	values := &c.Requests
	if limit {
		values = &c.Limits
	}
	if *values == nil {
		*values = map[corev1.ResourceName]*resource.Quantity{}
	}
	if _, recorded := (*values)[name]; recorded {
		return false
	}
	if value != nil {
		q := value.DeepCopy()
		value = &q
	}
	(*values)[name] = value
	return true
}

// recordChanges records the original value of the cpu and memory requests and limits that differ
// between before and after, the resources of container before and after a change. It reports
// whether the record changed.
func (o originalResources) recordChanges(container string, before, after corev1.ResourceRequirements) bool {
	changed := false
	for _, name := range recordedResourceNames {
		for _, limit := range []bool{false, true} {
			beforeList, afterList := before.Requests, after.Requests
			if limit {
				beforeList, afterList = before.Limits, after.Limits
			}
			b, wasSet := beforeList[name]
			a, isSet := afterList[name]
			if wasSet == isSet && (!wasSet || b.Cmp(a) == 0) {
				continue
			}
			var original *resource.Quantity
			if wasSet {
				original = &b
			}
			changed = o.record(container, limit, name, original) || changed
		}
	}
	return changed
}
