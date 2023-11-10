// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package mutate

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	admCommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/metrics"
	"github.com/DataDog/datadog-agent/pkg/config"
	apiCommon "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// Env vars
	agentHostEnvVarName    = "DD_AGENT_HOST"
	ddEntityIDEnvVarName   = "DD_ENTITY_ID"
	traceURLEnvVarName     = "DD_TRACE_AGENT_URL"
	dogstatsdURLEnvVarName = "DD_DOGSTATSD_URL"

	// Config injection modes
	hostIP  = "hostip"
	socket  = "socket"
	service = "service"

	// Volume name
	datadogVolumeName = "datadog"
)

var (
	agentHostIPEnvVar = corev1.EnvVar{
		Name:  agentHostEnvVarName,
		Value: "",
		ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{
				FieldPath: "status.hostIP",
			},
		},
	}

	agentHostServiceEnvVar = corev1.EnvVar{
		Name:  agentHostEnvVarName,
		Value: config.Datadog.GetString("admission_controller.inject_config.local_service_name") + "." + apiCommon.GetMyNamespace() + ".svc.cluster.local",
	}

	ddEntityIDEnvVar = corev1.EnvVar{
		Name:  ddEntityIDEnvVarName,
		Value: "",
		ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{
				FieldPath: "metadata.uid",
			},
		},
	}

	traceURLSocketEnvVar = corev1.EnvVar{
		Name:  traceURLEnvVarName,
		Value: config.Datadog.GetString("admission_controller.inject_config.trace_agent_socket"),
	}

	dogstatsdURLSocketEnvVar = corev1.EnvVar{
		Name:  dogstatsdURLEnvVarName,
		Value: config.Datadog.GetString("admission_controller.inject_config.dogstatsd_socket"),
	}
)

// InjectConfig adds the DD_AGENT_HOST and DD_ENTITY_ID env vars to the pod template if they don't exist
func InjectConfig(rawPod []byte, _ string, ns string, _ *authenticationv1.UserInfo, dc dynamic.Interface, _ kubernetes.Interface) ([]byte, error) {
	return mutate(rawPod, ns, injectConfig, dc)
}

// injectConfig injects DD_AGENT_HOST and DD_ENTITY_ID into a pod template if needed
func injectConfig(pod *corev1.Pod, _ string, _ dynamic.Interface) error {
	var injectedConfig, injectedEntity bool
	defer func() {
		metrics.MutationAttempts.Inc(metrics.ConfigMutationType, strconv.FormatBool(injectedConfig || injectedEntity), "")
	}()

	if pod == nil {
		metrics.MutationErrors.Inc(metrics.ConfigMutationType, "nil pod", "")
		return errors.New("cannot inject config into nil pod")
	}

	if !shouldInject(pod) {
		return nil
	}

	mode := injectionMode(pod, config.Datadog.GetString("admission_controller.inject_config.mode"))
	switch mode {
	case hostIP:
		injectedConfig = injectEnv(pod, agentHostIPEnvVar)
	case service:
		injectedConfig = injectEnv(pod, agentHostServiceEnvVar)
	case socket:
		volume, volumeMount := buildVolume(datadogVolumeName, config.Datadog.GetString("admission_controller.inject_config.socket_path"), true)
		injectedVol := injectVolume(pod, volume, volumeMount)
		injectedEnv := injectEnv(pod, traceURLSocketEnvVar)
		injectedEnv = injectEnv(pod, dogstatsdURLSocketEnvVar) || injectedEnv
		injectedConfig = injectedEnv || injectedVol
	default:
		metrics.MutationErrors.Inc(metrics.ConfigMutationType, "unknown mode", "")
		return fmt.Errorf("invalid injection mode %q", mode)
	}

	injectedEntity = injectEnv(pod, ddEntityIDEnvVar)

	return nil
}

// injectionMode returns the injection mode based on the global mode and pod labels
func injectionMode(pod *corev1.Pod, globalMode string) string {
	if val, found := pod.GetLabels()[admCommon.InjectionModeLabelKey]; found {
		mode := strings.ToLower(val)
		switch mode {
		case hostIP, service, socket:
			return mode
		default:
			log.Warnf("Invalid label value '%s=%s' on pod %s should be either 'hostip', 'service' or 'socket', defaulting to %q", admCommon.InjectionModeLabelKey, val, podString(pod), globalMode)
			return globalMode
		}
	}

	return globalMode
}

func buildVolume(volumeName, path string, readOnly bool) (corev1.Volume, corev1.VolumeMount) {
	pathType := corev1.HostPathDirectoryOrCreate
	volume := corev1.Volume{
		Name: volumeName,
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: path,
				Type: &pathType,
			},
		},
	}

	volumeMount := corev1.VolumeMount{
		Name:      volumeName,
		MountPath: path,
		ReadOnly:  readOnly,
	}

	return volume, volumeMount
}
