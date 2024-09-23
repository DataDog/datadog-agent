// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package config implements the webhook that injects DD_AGENT_HOST and
// DD_ENTITY_ID into a pod template as needed
package config

import (
	"errors"
	"fmt"
	"strings"

	admiv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/admission"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	admCommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/metrics"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	apiCommon "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// Env vars
	agentHostEnvVarName      = "DD_AGENT_HOST"
	ddEntityIDEnvVarName     = "DD_ENTITY_ID"
	ddExternalDataEnvVarName = "DD_EXTERNAL_ENV"
	traceURLEnvVarName       = "DD_TRACE_AGENT_URL"
	dogstatsdURLEnvVarName   = "DD_DOGSTATSD_URL"
	podUIDEnvVarName         = "DD_INTERNAL_POD_UID"

	// External Data Prefixes
	// These prefixes are used to build the External Data Environment Variable.
	// This variable is then used for Origin Detection.
	externalDataInitPrefix          = "it-"
	externalDataContainerNamePrefix = "cn-"
	externalDataPodUIDPrefix        = "pu-"

	// Config injection modes
	hostIP  = "hostip"
	socket  = "socket"
	service = "service"

	// DatadogVolumeName is the name of the volume used to mount the sockets when the volume source is a directory
	DatadogVolumeName = "datadog"

	// TraceAgentSocketVolumeName is the name of the volume used to mount the trace agent socket
	TraceAgentSocketVolumeName = "datadog-trace-agent"

	// DogstatsdSocketVolumeName is the name of the volume used to mount the dogstatsd socket
	DogstatsdSocketVolumeName = "datadog-dogstatsd"

	webhookName = "agent_config"
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
		Value: pkgconfigsetup.Datadog().GetString("admission_controller.inject_config.local_service_name") + "." + apiCommon.GetMyNamespace() + ".svc.cluster.local",
	}

	defaultDdEntityIDEnvVar = corev1.EnvVar{
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
		Value: pkgconfigsetup.Datadog().GetString("admission_controller.inject_config.trace_agent_socket"),
	}

	dogstatsdURLSocketEnvVar = corev1.EnvVar{
		Name:  dogstatsdURLEnvVarName,
		Value: pkgconfigsetup.Datadog().GetString("admission_controller.inject_config.dogstatsd_socket"),
	}
)

// Webhook is the webhook that injects DD_AGENT_HOST and DD_ENTITY_ID into a pod
type Webhook struct {
	name            string
	isEnabled       bool
	endpoint        string
	resources       []string
	operations      []admiv1.OperationType
	mode            string
	wmeta           workloadmeta.Component
	injectionFilter common.InjectionFilter
}

// NewWebhook returns a new Webhook
func NewWebhook(wmeta workloadmeta.Component, injectionFilter common.InjectionFilter) *Webhook {
	return &Webhook{
		name:            webhookName,
		isEnabled:       pkgconfigsetup.Datadog().GetBool("admission_controller.inject_config.enabled"),
		endpoint:        pkgconfigsetup.Datadog().GetString("admission_controller.inject_config.endpoint"),
		resources:       []string{"pods"},
		operations:      []admiv1.OperationType{admiv1.Create},
		mode:            pkgconfigsetup.Datadog().GetString("admission_controller.inject_config.mode"),
		wmeta:           wmeta,
		injectionFilter: injectionFilter,
	}
}

// Name returns the name of the webhook
func (w *Webhook) Name() string {
	return w.name
}

// IsEnabled returns whether the webhook is enabled
func (w *Webhook) IsEnabled() bool {
	return w.isEnabled
}

// Endpoint returns the endpoint of the webhook
func (w *Webhook) Endpoint() string {
	return w.endpoint
}

// Resources returns the kubernetes resources for which the webhook should
// be invoked
func (w *Webhook) Resources() []string {
	return w.resources
}

// Operations returns the operations on the resources specified for which
// the webhook should be invoked
func (w *Webhook) Operations() []admiv1.OperationType {
	return w.operations
}

// LabelSelectors returns the label selectors that specify when the webhook
// should be invoked
func (w *Webhook) LabelSelectors(useNamespaceSelector bool) (namespaceSelector *metav1.LabelSelector, objectSelector *metav1.LabelSelector) {
	return common.DefaultLabelSelectors(useNamespaceSelector)
}

// MutateFunc returns the function that mutates the resources
func (w *Webhook) MutateFunc() admission.WebhookFunc {
	return w.mutate
}

// mutate adds the DD_AGENT_HOST and DD_ENTITY_ID env vars to the pod template if they don't exist
func (w *Webhook) mutate(request *admission.MutateRequest) ([]byte, error) {
	return common.Mutate(request.Raw, request.Namespace, w.Name(), w.inject, request.DynamicClient)
}

// inject injects the following environment variables into the pod template:
// - DD_AGENT_HOST: the host IP of the node
// - DD_ENTITY_ID: the entity ID of the pod
// - DD_EXTERNAL_ENV: the External Data Environment Variable
func (w *Webhook) inject(pod *corev1.Pod, _ string, _ dynamic.Interface) (bool, error) {
	var injectedConfig, injectedEntity, injectedExternalEnv bool

	if pod == nil {
		return false, errors.New(metrics.InvalidInput)
	}

	if !w.injectionFilter.ShouldMutatePod(pod) {
		return false, nil
	}

	// Inject DD_AGENT_HOST
	switch injectionMode(pod, w.mode) {
	case hostIP:
		injectedConfig = common.InjectEnv(pod, agentHostIPEnvVar)
	case service:
		injectedConfig = common.InjectEnv(pod, agentHostServiceEnvVar)
	case socket:
		injectedVolumes := injectSocketVolumes(pod)
		injectedEnv := common.InjectEnv(pod, traceURLSocketEnvVar)
		injectedEnv = common.InjectEnv(pod, dogstatsdURLSocketEnvVar) || injectedEnv
		injectedConfig = injectedVolumes || injectedEnv
	default:
		log.Errorf("invalid injection mode %q", w.mode)
		return false, errors.New(metrics.InvalidInput)
	}

	injectedEntity = common.InjectEnv(pod, defaultDdEntityIDEnvVar)

	// Inject External Data Environment Variable
	injectedExternalEnv = injectExternalDataEnvVar(pod)

	return injectedConfig || injectedEntity || injectedExternalEnv, nil
}

// injectionMode returns the injection mode based on the global mode and pod labels
func injectionMode(pod *corev1.Pod, globalMode string) string {
	if val, found := pod.GetLabels()[admCommon.InjectionModeLabelKey]; found {
		mode := strings.ToLower(val)
		switch mode {
		case hostIP, service, socket:
			return mode
		default:
			log.Warnf("Invalid label value '%s=%s' on pod %s should be either 'hostip', 'service' or 'socket', defaulting to %q", admCommon.InjectionModeLabelKey, val, common.PodString(pod), globalMode)
			return globalMode
		}
	}

	return globalMode
}

// buildExternalEnv generate an External Data environment variable.
func buildExternalEnv(container *corev1.Container, init bool) (corev1.EnvVar, error) {
	return corev1.EnvVar{
		Name:  ddExternalDataEnvVarName,
		Value: fmt.Sprintf("%s%t,%s%s,%s$(%s)", externalDataInitPrefix, init, externalDataContainerNamePrefix, container.Name, externalDataPodUIDPrefix, podUIDEnvVarName),
	}, nil
}

// injectExternalDataEnvVar injects the External Data environment variable.
// The format is: it-<init>,cn-<container_name>,pu-<pod_uid>
func injectExternalDataEnvVar(pod *corev1.Pod) (injected bool) {
	// Inject External Data Environment Variable for the pod
	injected = common.InjectDynamicEnv(pod, buildExternalEnv)

	// Inject Internal Pod UID
	injected = common.InjectEnv(pod, corev1.EnvVar{
		Name: podUIDEnvVarName,
		ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{
				FieldPath: "metadata.uid",
			},
		},
	}) || injected

	return
}

func buildVolume(volumeName, path string, hostpathType corev1.HostPathType, readOnly bool) (corev1.Volume, corev1.VolumeMount) {
	volume := corev1.Volume{
		Name: volumeName,
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: path,
				Type: &hostpathType,
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

// injectSocketVolumes injects the volumes for the dogstatsd and trace agent
// sockets.
//
// The type of the volume injected can be either a directory or a socket
// depending on the configuration. They offer different trade-offs. Using a
// socket ensures no lost traces or dogstatsd metrics but can cause the pod to
// wait if the agent has issues that prevent it from creating the sockets.
//
// This function returns true if at least one volume was injected.
func injectSocketVolumes(pod *corev1.Pod) bool {
	var injectedVolNames []string

	if pkgconfigsetup.Datadog().GetBool("admission_controller.inject_config.type_socket_volumes") {
		volumes := map[string]string{
			DogstatsdSocketVolumeName: strings.TrimPrefix(
				pkgconfigsetup.Datadog().GetString("admission_controller.inject_config.dogstatsd_socket"), "unix://",
			),
			TraceAgentSocketVolumeName: strings.TrimPrefix(
				pkgconfigsetup.Datadog().GetString("admission_controller.inject_config.trace_agent_socket"), "unix://",
			),
		}

		for volumeName, volumePath := range volumes {
			volume, volumeMount := buildVolume(volumeName, volumePath, corev1.HostPathSocket, true)
			injectedVol := common.InjectVolume(pod, volume, volumeMount)
			if injectedVol {
				injectedVolNames = append(injectedVolNames, volumeName)
			}
		}
	} else {
		volume, volumeMount := buildVolume(
			DatadogVolumeName,
			pkgconfigsetup.Datadog().GetString("admission_controller.inject_config.socket_path"),
			corev1.HostPathDirectoryOrCreate,
			true,
		)
		injectedVol := common.InjectVolume(pod, volume, volumeMount)
		if injectedVol {
			injectedVolNames = append(injectedVolNames, DatadogVolumeName)
		}
	}

	for _, volName := range injectedVolNames {
		common.MarkVolumeAsSafeToEvictForAutoscaler(pod, volName)
	}

	return len(injectedVolNames) > 0
}
