// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package mutate

import (
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"gomodules.xyz/jsonpatch/v3"
	admiv1beta1 "k8s.io/api/admission/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"
)

type mutateFunc func(*corev1.Pod, string, dynamic.Interface) error

// mutate handles mutating pods and encoding and decoding admission
// requests and responses for the public mutate functions
func mutate(req *admiv1beta1.AdmissionRequest, m mutateFunc, dc dynamic.Interface) (*admiv1beta1.AdmissionResponse, error) {
	var pod corev1.Pod
	if err := json.Unmarshal(req.Object.Raw, &pod); err != nil {
		return nil, fmt.Errorf("failed to decode raw object: %v", err)
	}

	if err := m(&pod, req.Namespace, dc); err != nil {
		return nil, err
	}

	bytes, err := json.Marshal(pod)
	if err != nil {
		return nil, fmt.Errorf("failed to encode the mutated Pod object: %v", err)
	}

	patchOperation, err := jsonpatch.CreatePatch(req.Object.Raw, bytes) // TODO: Try to generate the patch at the mutateFunc
	if err != nil {
		return nil, fmt.Errorf("failed to prepare the JSON patch: %v", err)
	}

	patchEncoded, err := json.Marshal(patchOperation)
	if err != nil {
		return nil, fmt.Errorf("failed to encode the JSON patch: %v", err)
	}

	return &admiv1beta1.AdmissionResponse{
		Allowed: true,
		Patch:   patchEncoded,
	}, nil
}

// contains returns whether EnvVar slice contains an env var with a given name
func contains(envs []corev1.EnvVar, name string) bool {
	for _, env := range envs {
		if env.Name == name {
			return true
		}
	}
	return false
}

// injectEnv injects an env var into a pod template if it doesn't exist
func injectEnv(pod *corev1.Pod, env corev1.EnvVar) bool {
	injected := false
	podStr := podString(pod)
	log.Debugf("Injecting env var '%s' into pod %s", env.Name, podStr)
	for i, ctr := range pod.Spec.Containers {
		if contains(ctr.Env, env.Name) {
			log.Debugf("Ignoring container '%s' in pod %s: env var '%s' already exist", ctr.Name, podStr, env.Name)
			continue
		}
		pod.Spec.Containers[i].Env = append(pod.Spec.Containers[i].Env, env)
		injected = true
	}
	return injected
}

// podString returns a string that helps identify the pod
func podString(pod *corev1.Pod) string {
	if pod.GetNamespace() == "" || pod.GetName() == "" {
		return fmt.Sprintf("with generate name %s", pod.GetGenerateName())
	}
	return fmt.Sprintf("%s/%s", pod.GetNamespace(), pod.GetName())
}
