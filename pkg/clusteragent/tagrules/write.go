// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package tagrules

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// annotationPatch is a merge-patch fragment for metadata.annotations: present
// keys are set (nil deletes the key), absent keys are untouched.
type annotationPatch map[string]*string

func set(patch annotationPatch, key, value string) {
	patch[key] = &value
}

func del(patch annotationPatch, key string) {
	patch[key] = nil
}

// adTagsAnnotationKey returns the AD tags annotation key for a container.
func adTagsAnnotationKey(container string) string {
	return fmt.Sprintf(ADContainerTagsAnnotationFormat, container)
}

// computePodAnnotations computes the annotation patch for a pod given the
// owned tag keys (tag key to value) after evaluation, including the flip-hold
// outcome. Previously owned keys come from the ledger annotation. User-set
// keys inside the AD tags JSON are preserved; a container whose annotation
// exists but is not a valid JSON object is left untouched and reported as an
// error, so controller writes never destroy user data.
func computePodAnnotations(pod *unstructured.Unstructured, owned map[string]string) (annotationPatch, error) {
	patch := annotationPatch{}

	previousOwned := parseManagedTagKeys(pod.GetAnnotations()[ManagedTagKeysAnnotation])
	stale := make([]string, 0, len(previousOwned))
	for _, key := range previousOwned {
		if _, stillOwned := owned[key]; !stillOwned {
			stale = append(stale, key)
		}
	}

	containers, err := containerNames(pod)
	if err != nil {
		return nil, err
	}
	if len(containers) == 0 && len(owned) == 0 && len(stale) == 0 {
		return patch, nil
	}

	for _, container := range containers {
		key := adTagsAnnotationKey(container)
		merged, skip, err := mergeADTags(pod.GetAnnotations()[key], owned, stale)
		if err != nil {
			return nil, fmt.Errorf("container %s of pod %s/%s: %w", container, pod.GetNamespace(), pod.GetName(), err)
		}
		if skip {
			continue
		}
		if merged == nil {
			// Nothing remains: drop the annotation if it exists.
			if _, exists := pod.GetAnnotations()[key]; exists {
				del(patch, key)
			}
			continue
		}
		encoded, err := json.Marshal(merged)
		if err != nil {
			return nil, fmt.Errorf("encoding tags annotation for container %s of pod %s/%s: %w", container, pod.GetNamespace(), pod.GetName(), err)
		}
		set(patch, key, string(encoded))
	}

	if len(owned) > 0 {
		ledger := formatManagedTagKeys(slices.Sorted(maps.Keys(owned)))
		set(patch, ManagedTagKeysAnnotation, ledger)
	} else if len(previousOwned) > 0 {
		// No rules own keys on this pod anymore: drop the ledger.
		del(patch, ManagedTagKeysAnnotation)
	}

	return patch, nil
}

// mergeADTags merges owned and stale keys into an existing AD tags annotation.
// Returns the merged map (nil when nothing remains), skip=true when the
// existing annotation must not be touched (absent, or unparseable user data).
func mergeADTags(raw string, owned map[string]string, stale []string) (map[string]string, bool, error) {
	if raw == "" {
		if len(owned) == 0 {
			return nil, false, nil
		}
		merged := make(map[string]string, len(owned))
		maps.Copy(merged, owned)
		return merged, false, nil
	}

	var existing map[string]any
	if err := json.Unmarshal([]byte(raw), &existing); err != nil {
		// Unparseable user data: never touch it.
		return nil, true, nil
	}

	merged := make(map[string]string, len(existing)+len(owned))
	for key, value := range existing {
		if slices.Contains(stale, key) {
			continue
		}
		if _, reowned := owned[key]; reowned {
			continue // Controller wins on drift: overwrite below.
		}
		merged[key] = fmt.Sprint(value)
	}
	maps.Copy(merged, owned)

	if len(merged) == 0 {
		return nil, false, nil
	}
	return merged, false, nil
}

// computeNodeAnnotations computes the annotation patch for a node. Owned keys
// are written under the reserved prefix; previously owned keys (ledger) that
// are no longer owned are deleted.
func computeNodeAnnotations(node *unstructured.Unstructured, owned map[string]string) (annotationPatch, error) {
	patch := annotationPatch{}

	previousOwned := parseManagedTagKeys(node.GetAnnotations()[ManagedTagKeysAnnotation])
	for key, value := range owned {
		set(patch, NodeTagAnnotationPrefix+key, value)
	}
	for _, key := range previousOwned {
		if _, stillOwned := owned[key]; !stillOwned {
			del(patch, NodeTagAnnotationPrefix+key)
		}
	}

	if len(owned) > 0 {
		ledger := formatManagedTagKeys(slices.Sorted(maps.Keys(owned)))
		set(patch, ManagedTagKeysAnnotation, ledger)
	} else if len(previousOwned) > 0 {
		del(patch, ManagedTagKeysAnnotation)
	}

	return patch, nil
}

// ownedTagValue returns the value the entity currently carries for an owned
// tag key, if any: the AD tags JSON for pods, the prefix annotation for nodes.
// Used by status aggregation.
func ownedTagValue(entity *unstructured.Unstructured, kind EntityKind, tagKey string) (string, bool, error) {
	annotations := entity.GetAnnotations()
	switch kind {
	case EntityPod:
		containers, err := containerNames(entity)
		if err != nil {
			return "", false, err
		}
		for _, container := range containers {
			raw := annotations[adTagsAnnotationKey(container)]
			if raw == "" {
				continue
			}
			var tags map[string]any
			if err := json.Unmarshal([]byte(raw), &tags); err != nil {
				continue
			}
			if value, ok := tags[tagKey]; ok {
				return fmt.Sprint(value), true, nil
			}
		}
		return "", false, nil
	case EntityNode:
		value, ok := annotations[NodeTagAnnotationPrefix+tagKey]
		return value, ok, nil
	default:
		return "", false, fmt.Errorf("unsupported entity kind %q", kind)
	}
}
