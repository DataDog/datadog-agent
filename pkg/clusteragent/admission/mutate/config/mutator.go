// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package config

import (
	"errors"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/metrics"
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	apiCommon "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

// MutatorConfig contains the settings for the config injector.
type MutatorConfig struct {
	csiEnabled        bool
	mode              string
	localServiceName  string
	traceAgentSocket  string
	dogStatsDSocket   string
	socketPath        string
	typeSocketVolumes bool
	csiDriver         string
}

// NewMutatorConfig instantiates the required settings for the mutator from the datadog config.
func NewMutatorConfig(datadogConfig config.Component) *MutatorConfig {
	return &MutatorConfig{
		csiEnabled:        datadogConfig.GetBool("csi.enabled"),
		mode:              datadogConfig.GetString("admission_controller.inject_config.mode"),
		localServiceName:  datadogConfig.GetString("admission_controller.inject_config.local_service_name"),
		traceAgentSocket:  datadogConfig.GetString("admission_controller.inject_config.trace_agent_socket"),
		dogStatsDSocket:   datadogConfig.GetString("admission_controller.inject_config.dogstatsd_socket"),
		socketPath:        datadogConfig.GetString("admission_controller.inject_config.socket_path"),
		typeSocketVolumes: datadogConfig.GetBool("admission_controller.inject_config.type_socket_volumes"),
		csiDriver:         datadogConfig.GetString("csi.driver"),
	}
}

// Mutator satisfies the common.Mutator interface for the config webhook.
type Mutator struct {
	config *MutatorConfig
	filter mutatecommon.MutationFilter
}

// NewMutator creates a new mutator for the config webhook.
func NewMutator(cfg *MutatorConfig, filter mutatecommon.MutationFilter) *Mutator {
	return &Mutator{
		config: cfg,
		filter: filter,
	}
}

// MutatePod implements the common.Mutator interface for the config webhook. It injects the following environment
// variables into the pod template:
//   - DD_AGENT_HOST: the host IP of the node
//   - DD_ENTITY_ID: the entity ID of the pod
//   - DD_EXTERNAL_ENV: the External Data Environment Variable
func (i *Mutator) MutatePod(pod *corev1.Pod, _ string, _ dynamic.Interface) (bool, error) {
	var injectedConfig, injectedEntity, injectedExternalEnv bool
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
			Value: i.config.localServiceName + "." + apiCommon.GetMyNamespace() + ".svc.cluster.local",
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
			Value: i.config.traceAgentSocket,
		}

		dogstatsdURLSocketEnvVar = corev1.EnvVar{
			Name:  dogstatsdURLEnvVarName,
			Value: i.config.dogStatsDSocket,
		}
	)

	if pod == nil {
		return false, errors.New(metrics.InvalidInput)
	}

	if !i.filter.ShouldMutatePod(pod) {
		return false, nil
	}

	// Inject DD_AGENT_HOST
	mode := injectionMode(pod, i.config.mode, i.config.csiEnabled)
	switch mode {
	case hostIP:
		injectedConfig = mutatecommon.InjectEnv(pod, agentHostIPEnvVar)
	case service:
		injectedConfig = mutatecommon.InjectEnv(pod, agentHostServiceEnvVar)
	case socket, csi:
		useCSI := (mode == csi)
		injectedVolumesOrVolumeMounts := i.injectSocketVolumes(pod, useCSI)
		injectedEnv := mutatecommon.InjectEnv(pod, traceURLSocketEnvVar)
		injectedEnv = mutatecommon.InjectEnv(pod, dogstatsdURLSocketEnvVar) || injectedEnv
		injectedConfig = injectedVolumesOrVolumeMounts || injectedEnv
	default:
		log.Errorf("invalid injection mode %q", i.config.mode)
		return false, errors.New(metrics.InvalidInput)
	}

	injectedEntity = mutatecommon.InjectEnv(pod, defaultDdEntityIDEnvVar)

	// Inject External Data Environment Variable
	injectedExternalEnv = injectExternalDataEnvVar(pod)

	return injectedConfig || injectedEntity || injectedExternalEnv, nil
}

// injectSocketVolumes injects the volumes for the dogstatsd and trace agent
// sockets.
//
// The type of the volume injected can be either a directory or a socket
// depending on the configuration. They offer different trade-offs. Using a
// socket ensures no lost traces or dogstatsd metrics but can cause the pod to
// wait if the agent has issues that prevent it from creating the sockets.
//
// If withCSI is true, a CSI volume is injected. Otherwise, a normal hostpath
// volume is injected.
//
// This function returns true if at least one volume or volume mount was injected
func (i *Mutator) injectSocketVolumes(pod *corev1.Pod, withCSI bool) bool {
	var injectedVolNames []string
	var injectedVolumeMount bool

	if shouldUseSocketVolumeType(pod, i.config.typeSocketVolumes) {
		volumes := map[string]struct {
			socketpath    string
			csiVolumeType csiInjectionType
		}{
			DogstatsdSocketVolumeName: {
				socketpath: strings.TrimPrefix(
					i.config.dogStatsDSocket, "unix://",
				),
				csiVolumeType: csiDSDSocket,
			},
			TraceAgentSocketVolumeName: {
				socketpath: strings.TrimPrefix(
					i.config.traceAgentSocket, "unix://",
				),
				csiVolumeType: csiAPMSocket,
			},
		}

		for volumeName, volumeProps := range volumes {
			var volume corev1.Volume
			var volumeMount corev1.VolumeMount

			if withCSI {
				volume, volumeMount = buildCSIVolume(volumeName, volumeProps.socketpath, volumeProps.csiVolumeType, true, i.config.csiDriver)
			} else {
				volume, volumeMount = buildHostPathVolume(volumeName, volumeProps.socketpath, corev1.HostPathSocket, true)
			}
			var injectedVol bool
			injectedVol, injectedVolumeMount = mutatecommon.InjectVolume(pod, volume, volumeMount)
			if injectedVol {
				injectedVolNames = append(injectedVolNames, volumeName)
			}
		}
	} else {
		var volume corev1.Volume
		var volumeMount corev1.VolumeMount
		if withCSI {
			// csiDSDSocketDirectory is used because we can't mount two different volumes at the same mount path
			// this is a limitation in the configuration of the admission controller
			volume, volumeMount = buildCSIVolume(DatadogVolumeName, i.config.socketPath, csiDSDSocketDirectory, true, i.config.csiDriver)
		} else {
			volume, volumeMount = buildHostPathVolume(
				DatadogVolumeName,
				i.config.socketPath,
				corev1.HostPathDirectoryOrCreate,
				true,
			)
		}
		var injectedVol bool
		injectedVol, injectedVolumeMount = mutatecommon.InjectVolume(pod, volume, volumeMount)
		if injectedVol {
			injectedVolNames = append(injectedVolNames, DatadogVolumeName)
		}
	}

	for _, volName := range injectedVolNames {
		mutatecommon.MarkVolumeAsSafeToEvictForAutoscaler(pod, volName)
	}

	return len(injectedVolNames) > 0 || injectedVolumeMount
}

// injectionMode returns the injection mode based on the global mode and pod labels
func injectionMode(pod *corev1.Pod, globalMode string, csiEnabled bool) string {
	decidedMode := globalMode

	if val, found := pod.GetLabels()[common.InjectionModeLabelKey]; found {
		mode := strings.ToLower(val)
		switch mode {
		case hostIP, service, socket, csi:
			decidedMode = mode
		default:
			log.Warnf("Invalid label value '%s=%s' on pod %s should be either 'hostip', 'service', 'socket' or 'csi', defaulting to %q", common.InjectionModeLabelKey, val, mutatecommon.PodString(pod), globalMode)
			decidedMode = globalMode
		}
	}

	if decidedMode == csi && !csiEnabled {
		log.Warnf("Unable to use CSI mode because CSI is disabled, defaulting to 'socket'")
		decidedMode = socket
	}

	return decidedMode
}

// shouldUseSocketVolumeType determines if socket volume type should be used for the pod under mutation.
func shouldUseSocketVolumeType(pod *corev1.Pod, globalTypeSocketVolumes bool) bool {
	if val, found := pod.GetLabels()[common.TypeSocketVolumesLabelKey]; found {
		normalisedValue := strings.ToLower(val)

		if normalisedValue != "true" && normalisedValue != "false" {
			log.Warnf("Invalid value for %q: %q. Expected values are `true` and `false`.", common.TypeSocketVolumesLabelKey, normalisedValue)
			return globalTypeSocketVolumes
		}

		return normalisedValue == "true"
	}

	return globalTypeSocketVolumes
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
	injected = mutatecommon.InjectDynamicEnv(pod, buildExternalEnv)

	// Inject Internal Pod UID
	injected = mutatecommon.InjectEnv(pod, corev1.EnvVar{
		Name: podUIDEnvVarName,
		ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{
				FieldPath: "metadata.uid",
			},
		},
	}) || injected

	return
}

func buildHostPathVolume(volumeName, path string, hostpathType corev1.HostPathType, readOnly bool) (corev1.Volume, corev1.VolumeMount) {
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

func buildCSIVolume(volumeName, path string, csiVolumeType csiInjectionType, readOnly bool, csiDriver string) (corev1.Volume, corev1.VolumeMount) {
	volume := corev1.Volume{
		Name: volumeName,
		VolumeSource: corev1.VolumeSource{
			CSI: &corev1.CSIVolumeSource{
				Driver:   csiDriver,
				ReadOnly: pointer.Ptr(readOnly),
				VolumeAttributes: map[string]string{
					"type": string(csiVolumeType),
				},
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
