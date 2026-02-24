// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package patcher

// PatchOperation represents a single mutation to be included in a patch.
// Operations are composable — multiple operations can be combined in a
// PatchIntent and their results are deep-merged into a single patch body.
type PatchOperation interface {
	// build returns a nested map fragment that will be deep-merged into
	// the final patch body. For example, setting a metadata annotation
	// returns {"metadata": {"annotations": {"key": "value"}}}.
	build() map[string]interface{}
}

// --- Metadata-level operations  ---

// setMetadataAnnotations sets annotations on .metadata.annotations.
// Values can be strings (set) or nil (delete).
type setMetadataAnnotations struct {
	annotations map[string]interface{}
}

// SetMetadataAnnotations creates an operation that sets (or deletes) annotations
// on the resource's metadata. Pass nil values to delete annotations.
func SetMetadataAnnotations(annotations map[string]interface{}) PatchOperation {
	return &setMetadataAnnotations{annotations: annotations}
}

func (o *setMetadataAnnotations) build() map[string]interface{} {
	return map[string]interface{}{
		"metadata": map[string]interface{}{
			"annotations": o.annotations,
		},
	}
}

// setMetadataLabels sets labels on .metadata.labels.
type setMetadataLabels struct {
	labels map[string]interface{}
}

// SetMetadataLabels creates an operation that sets (or deletes) labels
// on the resource's metadata. Pass nil values to delete labels.
func SetMetadataLabels(labels map[string]interface{}) PatchOperation {
	return &setMetadataLabels{labels: labels}
}

func (o *setMetadataLabels) build() map[string]interface{} {
	return map[string]interface{}{
		"metadata": map[string]interface{}{
			"labels": o.labels,
		},
	}
}

// --- Pod template operations (on pod controllers like Deployments, StatefulSets, Rollouts) ---

// setPodTemplateAnnotations sets annotations on .spec.template.metadata.annotations.
type setPodTemplateAnnotations struct {
	annotations map[string]interface{}
}

// SetPodTemplateAnnotations creates an operation that sets annotations on
// the pod template.
func SetPodTemplateAnnotations(annotations map[string]interface{}) PatchOperation {
	return &setPodTemplateAnnotations{annotations: annotations}
}

func (o *setPodTemplateAnnotations) build() map[string]interface{} {
	return map[string]interface{}{
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"annotations": o.annotations,
				},
			},
		},
	}
}

// setPodTemplateLabels sets labels on .spec.template.metadata.labels.
type setPodTemplateLabels struct {
	labels map[string]interface{}
}

// SetPodTemplateLabels creates an operation that sets labels on the pod
// template.
func SetPodTemplateLabels(labels map[string]interface{}) PatchOperation {
	return &setPodTemplateLabels{labels: labels}
}

func (o *setPodTemplateLabels) build() map[string]interface{} {
	return map[string]interface{}{
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": o.labels,
				},
			},
		},
	}
}

// deletePodTemplateAnnotations removes annotations from .spec.template.metadata.annotations
// by setting them to nil in a merge patch.
type deletePodTemplateAnnotations struct {
	keys []string
}

// DeletePodTemplateAnnotations creates an operation that removes the
// specified annotations from the pod template.
func DeletePodTemplateAnnotations(keys []string) PatchOperation {
	return &deletePodTemplateAnnotations{keys: keys}
}

func (o *deletePodTemplateAnnotations) build() map[string]interface{} {
	annots := make(map[string]interface{}, len(o.keys))
	for _, k := range o.keys {
		annots[k] = nil
	}
	return map[string]interface{}{
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"annotations": annots,
				},
			},
		},
	}
}

// --- Pod container operations ---

// ContainerResourcePatch describes the resource changes for a single container.
type ContainerResourcePatch struct {
	// Name is the container name
	Name string
	// Requests is the desired resource requests (e.g. {"cpu": "250m", "memory": "512Mi"}).
	Requests map[string]string
	// Limits is the desired resource limits (e.g. {"cpu": "500m", "memory": "1Gi"}).
	Limits map[string]string
}

// setContainerResources sets spec.containers[*].resources for the listed containers
type setContainerResources struct {
	containers []ContainerResourcePatch
}

// SetContainerResources creates an operation that sets resource requests and limits
// for the named containers via a strategic merge patch on spec.containers
func SetContainerResources(containers []ContainerResourcePatch) PatchOperation {
	return &setContainerResources{containers: containers}
}

func (o *setContainerResources) build() map[string]interface{} {
	containerList := make([]interface{}, 0, len(o.containers))
	for _, c := range o.containers {
		resources := map[string]interface{}{}
		if len(c.Requests) > 0 {
			requests := make(map[string]interface{}, len(c.Requests))
			for k, v := range c.Requests {
				requests[k] = v
			}
			resources["requests"] = requests
		}

		if len(c.Limits) > 0 {
			limits := make(map[string]interface{}, len(c.Limits))
			for k, v := range c.Limits {
				limits[k] = v
			}
			resources["limits"] = limits
		}

		containerList = append(containerList, map[string]interface{}{
			"name":      c.Name,
			"resources": resources,
		})
	}
	return map[string]interface{}{
		"spec": map[string]interface{}{
			"containers": containerList,
		},
	}
}
