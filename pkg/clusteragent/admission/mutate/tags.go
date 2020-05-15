// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package mutate

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"

	admiv1beta1 "k8s.io/api/admission/v1beta1"
	corev1 "k8s.io/api/core/v1"
)

var labelsToEnv = map[string]string{
	kubernetes.EnvTagLabelKey:     kubernetes.EnvTagEnvVar,
	kubernetes.ServiceTagLabelKey: kubernetes.ServiceTagEnvVar,
	kubernetes.VersionTagLabelKey: kubernetes.VersionTagEnvVar,
}

// InjectTags adds the DD_ENV, DD_VERSION, DD_SERVICE env vars to
// the pod template from pod and higher-level resource labels
func InjectTags(req *admiv1beta1.AdmissionRequest) (*admiv1beta1.AdmissionResponse, error) {
	return mutate(req, injectTags)
}

// injectTags injects DD_ENV, DD_VERSION, DD_SERVICE
// env vars into a pod template if needed
func injectTags(pod *corev1.Pod) error {
	if pod == nil {
		return errors.New("cannot inject tags into nil pod")
	}
	injectTagsFromLabels(pod)
	return nil
}

// injectTagsFromLabels looks for standard tags in pod labels
// and injects them as environment variables if found
func injectTagsFromLabels(pod *corev1.Pod) {
	for l, envName := range labelsToEnv {
		if tagValue, found := pod.GetLabels()[l]; found {
			env := corev1.EnvVar{
				Name:  envName,
				Value: tagValue,
			}
			injectEnv(pod, env)
		}
	}
}
