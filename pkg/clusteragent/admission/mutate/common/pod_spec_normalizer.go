// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package common

import (
	"reflect"
	"slices"

	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// NormalizePodSpec resolves any pod spec issues
func NormalizePodSpec(pod *corev1.Pod) error {
	normalizedVolumes, err := normalizeVolumes(pod.Spec.Volumes)
	if err != nil {
		return err
	}

	pod.Spec.Volumes = normalizedVolumes
	return nil
}

func normalizeVolumes(volumes []corev1.Volume) ([]corev1.Volume, error) {
	seen := make(map[string]int, len(volumes))
	var duplicates []int // Track duplicates in ascending order

	// Iterate over volumes and identify duplicates
	for i, vol := range volumes {
		if j, found := seen[vol.Name]; found {
			// Check that the identified duplicate is an _exact_ match to the previous
			if !reflect.DeepEqual(vol, volumes[j]) {
				return volumes, log.Errorf("detected multiple volumes with the name \"%s\" but some fields differ, cannot normalize", vol.Name)
			}
			log.Warnf("detected multiple entries for volume \"%s\", normalizing the pod spec", vol.Name)
			duplicates = append(duplicates, i)
		} else {
			seen[vol.Name] = i
		}
	}

	// Iterate in reverse (to avoid index shifting) and delete any duplicates
	for i := len(duplicates) - 1; i >= 0; i-- {
		volumes = slices.Delete(volumes, duplicates[i], duplicates[i]+1)
	}

	return volumes, nil
}
