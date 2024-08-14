// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	corev1 "k8s.io/api/core/v1"
)

type libRequirement struct {
	envVars               []envVar
	volumeMounts          []volumeMount
	initContainers        []initContainer
	volumes               []volume
	initContainerMutators []containerMutator
}

func (reqs libRequirement) injectPod(pod *corev1.Pod, ctrName string) error {
	for i, ctr := range pod.Spec.Containers {
		if ctrName != "" && ctrName != ctr.Name {
			continue
		}

		for _, v := range reqs.envVars {
			if err := v.mutateContainer(&ctr); err != nil {
				return err
			}
		}

		for _, v := range reqs.volumeMounts {
			if err := v.mutateContainer(&ctr); err != nil {
				return err
			}
		}

		pod.Spec.Containers[i] = ctr
	}

	for _, i := range reqs.initContainers {
		mutator := i
		mutator.Mutators = reqs.initContainerMutators
		if err := mutator.mutatePod(pod); err != nil {
			return err
		}
	}

	for _, v := range reqs.volumes {
		if err := v.mutatePod(pod); err != nil {
			return err
		}
	}

	return nil
}
