// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"encoding/json"

	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

// basicConfig returns the default tracing config to inject into application pods
// when no other config has been provided.
func basicConfig() common.LibConfig {
	return common.LibConfig{
		Tracing:        pointer.Ptr(true),
		LogInjection:   pointer.Ptr(true),
		HealthMetrics:  pointer.Ptr(true),
		RuntimeMetrics: pointer.Ptr(true),
	}
}

type basicLibConfigInjector struct{}

func (basicLibConfigInjector) mutatePod(pod *corev1.Pod) error {
	libConfig := basicConfig()
	for _, env := range libConfig.ToEnvs() {
		_ = mutatecommon.InjectEnv(pod, env)
	}

	return nil
}

// injectV1LibAnnotations extracts "v1" library injection annotations from a pod and
// adds the required environment variables for the language to the pod.
func injectV1LibAnnotations(pod *corev1.Pod, lang language) error {
	a, ok := pod.GetAnnotations()[lang.libConfigV1AnnotationKey()]
	if !ok {
		// annotation not found, do nothing
		return nil
	}

	lc, err := parseConfigJSON(a)
	if err != nil {
		return err
	}

	for _, env := range lc.ToEnvs() {
		_ = mutatecommon.InjectEnv(pod, env)
	}

	return nil
}

func parseConfigJSON(in string) (common.LibConfig, error) {
	var c common.LibConfig
	return c, json.Unmarshal([]byte(in), &c)
}
