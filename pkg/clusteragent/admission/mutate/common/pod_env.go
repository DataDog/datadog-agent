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

// Environment names need not be unique, and their order controls expansion of
// references to preceding variables. Replace env lists during strategic merge;
// preservePodEnvs merges their individual occurrences with the original JSON.
type podPatchMeta struct{ strategicpatch.LookupPatchMeta }

func (m podPatchMeta) LookupPatchMetadataForStruct(key string) (strategicpatch.LookupPatchMeta, strategicpatch.PatchMeta, error) {
	schema, meta, err := m.LookupPatchMeta.LookupPatchMetadataForStruct(key)
	return podPatchMeta{schema}, meta, err
}

func (m podPatchMeta) LookupPatchMetadataForSlice(key string) (strategicpatch.LookupPatchMeta, strategicpatch.PatchMeta, error) {
	schema, meta, err := m.LookupPatchMeta.LookupPatchMetadataForSlice(key)
	if key == "env" {
		meta.SetPatchStrategies(nil)
	}
	return podPatchMeta{schema}, meta, err
}

type envChange struct {
	beforeIndex, afterIndex int
	before, after           []corev1.EnvVar
}

func changedContainerEnvs[T any](before, after []T, env func(T) (string, []corev1.EnvVar)) []envChange {
	if reflect.DeepEqual(before, after) {
		return nil
	}
	indices := make(map[string]int, len(before))
	for i, container := range before {
		name, _ := env(container)
		indices[name] = i
	}
	var changes []envChange
	for i, container := range after {
		name, updated := env(container)
		j, found := indices[name]
		if !found || len(updated) == 0 {
			continue // New containers and whole-list deletion need no preservation.
		}
		_, original := env(before[j])
		if !reflect.DeepEqual(original, updated) {
			changes = append(changes, envChange{j, i, original, updated})
		}
	}
	return changes
}

func preservePodEnvs(raw, merged []byte, before, after *corev1.Pod) ([]byte, error) {
	containerEnv := func(c corev1.Container) (string, []corev1.EnvVar) { return c.Name, c.Env }
	changes := map[string][]envChange{
		"containers":     changedContainerEnvs(before.Spec.Containers, after.Spec.Containers, containerEnv),
		"initContainers": changedContainerEnvs(before.Spec.InitContainers, after.Spec.InitContainers, containerEnv),
		"ephemeralContainers": changedContainerEnvs(before.Spec.EphemeralContainers, after.Spec.EphemeralContainers,
			func(c corev1.EphemeralContainer) (string, []corev1.EnvVar) { return c.Name, c.Env }),
	}
	if len(changes["containers"])+len(changes["initContainers"])+len(changes["ephemeralContainers"]) == 0 {
		return merged, nil
	}
	var original struct {
		Spec json.RawMessage `json:"spec"`
	}
	if err := json.Unmarshal(raw, &original); err != nil {
		return nil, err
	}
	var inputSpec map[string]json.RawMessage
	if err := json.Unmarshal(original.Spec, &inputSpec); err != nil {
		return nil, err
	}
	var output, spec map[string]json.RawMessage
	if err := json.Unmarshal(merged, &output); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(output["spec"], &spec); err != nil {
		return nil, err
	}
	for field, entries := range changes {
		if len(entries) == 0 {
			continue
		}
		var rawContainers []json.RawMessage
		if err := json.Unmarshal(inputSpec[field], &rawContainers); err != nil {
			return nil, err
		}
		var containers []map[string]json.RawMessage
		if err := json.Unmarshal(spec[field], &containers); err != nil {
			return nil, err
		}
		for _, change := range entries {
			if change.beforeIndex >= len(rawContainers) || change.afterIndex >= len(containers) {
				return nil, errors.New("raw and typed container counts differ")
			}
			var originalContainer struct {
				Env []json.RawMessage `json:"env"`
			}
			if err := json.Unmarshal(rawContainers[change.beforeIndex], &originalContainer); err != nil {
				return nil, err
			}
			if len(originalContainer.Env) != len(change.before) {
				return nil, errors.New("raw and typed environment counts differ")
			}
			matches, err := matchListEntries(change.before, change.after, func(e corev1.EnvVar) string { return e.Name })
			if err != nil {
				return nil, err
			}
			envs := make([]json.RawMessage, len(change.after))
			for i, env := range change.after {
				if j := matches[i]; j >= 0 {
					envs[i], err = mergeListEntry(originalContainer.Env[j], change.before[j], env, corev1.EnvVar{})
				} else {
					envs[i], err = json.Marshal(env)
				}
				if err != nil {
					return nil, err
				}
			}
			encoded, err := json.Marshal(envs)
			if err != nil {
				return nil, err
			}
			containers[change.afterIndex]["env"] = encoded
		}
		encoded, err := json.Marshal(containers)
		if err != nil {
			return nil, err
		}
		spec[field] = encoded
	}
	encoded, err := json.Marshal(spec)
	if err != nil {
		return nil, err
	}
	output["spec"] = encoded
	return json.Marshal(output)
}
