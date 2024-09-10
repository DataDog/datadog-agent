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
	if name, err := getServiceNameFromPod(pod); err == nil {
		// Set service name if it can be derived from a pod
		libConfig.ServiceName = &name
	}
	for _, env := range libConfig.ToEnvs() {
		_ = mutatecommon.InjectEnv(pod, env)
	}

	return nil
}

type libConfigInjector struct{}

func (l *libConfigInjector) podMutator(lang language) podMutator {
	return podMutatorFunc(func(pod *corev1.Pod) error {
		c, err := lang.libConfigAnnotationExtractor().extract(pod)
		if err != nil {
			if isErrAnnotationNotFound(err) {
				return nil
			}

			return err
		}

		for _, env := range c.ToEnvs() {
			_ = mutatecommon.InjectEnv(pod, env)
		}

		return nil
	})
}

// injectLibConfig injects additional library configuration extracted from pod annotations
func injectLibConfig(pod *corev1.Pod, lang language) error {
	return (&libConfigInjector{}).podMutator(lang).mutatePod(pod)
}

func parseConfigJSON(in string) (common.LibConfig, error) {
	var c common.LibConfig
	return c, json.Unmarshal([]byte(in), &c)
}
