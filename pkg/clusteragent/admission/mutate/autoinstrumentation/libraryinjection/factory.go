// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package libraryinjection

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/annotation"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// InjectionMode represents the method used to inject APM libraries into pods.
type InjectionMode string

const (
	// InjectionModeAuto determines the best injection mode
	InjectionModeAuto InjectionMode = "auto"

	// InjectionModeInitContainer uses init containers to copy library files into an EmptyDir volume.
	// This is the traditional injection method.
	InjectionModeInitContainer InjectionMode = "init_container"

	// InjectionModeCSI uses the Datadog CSI driver to mount library files directly into the pod.
	InjectionModeCSI InjectionMode = "csi"

	// InjectionModeImageVolume uses an image volume to mount library files directly into the pod.
	InjectionModeImageVolume InjectionMode = "image_volume"
)

// ProviderFactory holds the default injection mode and creates providers on demand.
type ProviderFactory struct {
	defaultMode InjectionMode
}

// NewProviderFactory creates a new provider factory with the specified default mode.
func NewProviderFactory(defaultMode InjectionMode) *ProviderFactory {
	return &ProviderFactory{
		defaultMode: defaultMode,
	}
}

// GetProviderForPod returns the appropriate injection provider for a pod.
// It checks for the injection mode annotation on the pod and returns the corresponding provider.
// If no annotation is present or the value is invalid, it returns the default provider.
func (f *ProviderFactory) GetProviderForPod(pod *corev1.Pod, cfg LibraryInjectionConfig) LibraryInjectionProvider {
	mode := f.defaultMode

	if modeStr, ok := annotation.Get(pod, annotation.InjectionMode); ok && modeStr != "" {
		mode = InjectionMode(modeStr)
	}

	switch mode {
	default:
		log.Warnf("Unknown injection mode %q for pod %s/%s, using 'auto'", mode, pod.Namespace, pod.Name)
		fallthrough
	case InjectionModeAuto:
		return NewAutoProvider(cfg)
	case InjectionModeInitContainer:
		return NewInitContainerProvider(cfg)
	case InjectionModeCSI:
		return NewCSIProvider(cfg)
	case InjectionModeImageVolume:
		if !IsImageVolumeSupported(cfg.KubeServerVersion) {
			err := fmt.Errorf("image volume provider requires kubernetes version %s or higher", minImageVolumeKubeVersion)
			log.Warnf("%v; stopping injection for pod %s/%s", err, pod.Namespace, pod.Name)
			return newNoopProvider(err)
		}
		return NewImageVolumeProvider(cfg)
	}
}
