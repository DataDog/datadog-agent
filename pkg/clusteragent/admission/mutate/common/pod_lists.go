// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package common

import (
	"encoding/json"
	"errors"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
)

// preservePodLists handles lists whose strategic merge semantics discard fields
// on surviving entries: atomic tolerations and volumes with retainKeys.
func preservePodLists(raw, merged []byte, before, after *corev1.Pod) ([]byte, error) {
	volumesChanged := len(after.Spec.Volumes) > 0 && !reflect.DeepEqual(before.Spec.Volumes, after.Spec.Volumes)
	tolerationsChanged := len(after.Spec.Tolerations) > 0 && !reflect.DeepEqual(before.Spec.Tolerations, after.Spec.Tolerations)
	if !volumesChanged && !tolerationsChanged {
		return merged, nil // Strategic merge already handles whole-list deletion.
	}
	var input struct {
		Spec struct {
			Volumes     []json.RawMessage `json:"volumes"`
			Tolerations []json.RawMessage `json:"tolerations"`
		} `json:"spec"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	var output, spec map[string]json.RawMessage
	if err := json.Unmarshal(merged, &output); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(output["spec"], &spec); err != nil {
		return nil, err
	}
	if volumesChanged {
		if len(input.Spec.Volumes) != len(before.Spec.Volumes) {
			return nil, errors.New("raw and typed volume counts differ")
		}
		indices := make(map[string]int, len(before.Spec.Volumes))
		for i, volume := range before.Spec.Volumes {
			if _, exists := indices[volume.Name]; exists || volume.Name == "" {
				return nil, errors.New("volume names must be nonempty and unique")
			}
			indices[volume.Name] = i
		}
		volumes := make([]json.RawMessage, len(after.Spec.Volumes))
		for i, volume := range after.Spec.Volumes {
			var err error
			if j, exists := indices[volume.Name]; exists {
				// Diff a Volume directly, avoiding the containing list's retainKeys
				// directive. Known fields removed by the mutator still become null.
				volumes[i], err = mergeListEntry(input.Spec.Volumes[j], before.Spec.Volumes[j], volume, corev1.Volume{})
			} else {
				volumes[i], err = json.Marshal(volume)
			}
			if err != nil {
				return nil, err
			}
		}
		encoded, err := json.Marshal(volumes)
		if err != nil {
			return nil, err
		}
		spec["volumes"] = encoded
	}
	if tolerationsChanged {
		if len(input.Spec.Tolerations) != len(before.Spec.Tolerations) {
			return nil, errors.New("raw and typed toleration counts differ")
		}
		matches, err := matchTolerations(before.Spec.Tolerations, after.Spec.Tolerations)
		if err != nil {
			return nil, err
		}
		tolerations := make([]json.RawMessage, len(after.Spec.Tolerations))
		for i, toleration := range after.Spec.Tolerations {
			if j := matches[i]; j >= 0 {
				tolerations[i], err = mergeListEntry(input.Spec.Tolerations[j], before.Spec.Tolerations[j], toleration, corev1.Toleration{})
			} else {
				tolerations[i], err = json.Marshal(toleration)
			}
			if err != nil {
				return nil, err
			}
		}
		encoded, err := json.Marshal(tolerations)
		if err != nil {
			return nil, err
		}
		spec["tolerations"] = encoded
	}
	encoded, err := json.Marshal(spec)
	if err != nil {
		return nil, err
	}
	output["spec"] = encoded
	return json.Marshal(output)
}

func mergeListEntry(raw json.RawMessage, before, after, schema any) (json.RawMessage, error) {
	if reflect.DeepEqual(before, after) {
		return raw, nil
	}
	original, err := json.Marshal(before)
	if err != nil {
		return nil, err
	}
	mutated, err := json.Marshal(after)
	if err != nil {
		return nil, err
	}
	patch, err := strategicpatch.CreateTwoWayMergePatch(original, mutated, schema)
	if err != nil {
		return nil, err
	}
	return strategicpatch.StrategicMergePatch(raw, patch, schema)
}

// Reserve exact typed matches before matching edits by the toleration's selector.
// Tolerations have no unique identity; indistinguishable duplicates use original
// occurrence order. Index queues avoid rescanning the list for every match.
func matchTolerations(before, after []corev1.Toleration) ([]int, error) {
	type selector struct {
		key      string
		operator corev1.TolerationOperator
		effect   corev1.TaintEffect
	}
	identity := func(t corev1.Toleration) selector { return selector{t.Key, t.Operator, t.Effect} }
	exact := make(map[string][]int, len(before))
	bySelector := make(map[selector][]int, len(before))
	for i, toleration := range before {
		encoded, err := json.Marshal(toleration)
		if err != nil {
			return nil, err
		}
		exact[string(encoded)] = append(exact[string(encoded)], i)
		key := identity(toleration)
		bySelector[key] = append(bySelector[key], i)
	}
	matches := make([]int, len(after))
	used := make([]bool, len(before))
	for i, toleration := range after {
		matches[i] = -1
		encoded, err := json.Marshal(toleration)
		if err != nil {
			return nil, err
		}
		key := string(encoded)
		if queue := exact[key]; len(queue) > 0 {
			matches[i], used[queue[0]] = queue[0], true
			exact[key] = queue[1:]
		}
	}
	for i, toleration := range after {
		if matches[i] >= 0 {
			continue
		}
		key := identity(toleration)
		queue := bySelector[key]
		for len(queue) > 0 && used[queue[0]] {
			queue = queue[1:]
		}
		if len(queue) > 0 {
			matches[i] = queue[0]
			queue = queue[1:]
		}
		bySelector[key] = queue
	}
	return matches, nil
}
