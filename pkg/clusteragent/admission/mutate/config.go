// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package mutate

import (
	"errors"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/metrics"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	admiv1beta1 "k8s.io/api/admission/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"
)

const agentHostEnvVarName = "DD_AGENT_HOST"

var agentHostEnvVar = corev1.EnvVar{
	Name:  agentHostEnvVarName,
	Value: "",
	ValueFrom: &corev1.EnvVarSource{
		FieldRef: &corev1.ObjectFieldSelector{
			FieldPath: "status.hostIP",
		},
	},
}

// InjectConfig adds the DD_AGENT_HOST env var to the pod template if it doesn't exist
func InjectConfig(req *admiv1beta1.AdmissionRequest, dc dynamic.Interface) (*admiv1beta1.AdmissionResponse, error) {
	return mutate(req, injectConfig, dc)
}

// injectConfig injects DD_AGENT_HOST into a pod template if needed
func injectConfig(pod *corev1.Pod, _ string, _ dynamic.Interface) error {
	if pod == nil {
		metrics.MutationAttempts.Inc(metrics.ConfigMutationType, strconv.FormatBool(false))
		metrics.MutationErrors.Inc(metrics.ConfigMutationType, "nil pod")
		return errors.New("cannot inject config into nil pod")
	}
	injected := false
	if shouldInjectConf(pod) {
		injected = injectEnv(pod, agentHostEnvVar)
	}
	metrics.MutationAttempts.Inc(metrics.ConfigMutationType, strconv.FormatBool(injected))
	return nil
}

// shouldInjectConf returns whether the config should be injected
// based on the pod labels and the cluster agent config
func shouldInjectConf(pod *corev1.Pod) bool {
	if val, found := pod.GetLabels()[admission.EnabledLabelKey]; found {
		switch val {
		case "true":
			return true
		case "false":
			return false
		default:
			log.Warnf("Invalid label value '%s=%s' on pod %s should be either 'true' or 'false', ignoring it", admission.EnabledLabelKey, val, podString(pod))
			return false
		}
	}
	return config.Datadog.GetBool("admission_controller.mutate_unlabelled")
}
