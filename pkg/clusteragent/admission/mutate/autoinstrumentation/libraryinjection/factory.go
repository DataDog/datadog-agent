// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package libraryinjection

import (
	"context"

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
)

// ProviderFactory holds the default injection mode and creates providers on demand.
type ProviderFactory struct {
	defaultMode InjectionMode
	csiEnabled  bool
}

// NewProviderFactory creates a new provider factory with the specified default mode.
func NewProviderFactory(defaultMode InjectionMode, csiEnabled bool) *ProviderFactory {
	return &ProviderFactory{
		defaultMode: defaultMode,
		csiEnabled:  csiEnabled,
	}
}

// GetProviderForPod returns the appropriate injection provider for a pod.
// It checks for the injection mode annotation on the pod and returns the corresponding provider.
// If no annotation is present or the value is invalid, it returns the default provider.
// For CSI mode, it verifies that the CSI driver is available and SSI-enabled before using it.
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
		if !f.isCSIAvailable() {
			log.Warnf("CSI injection mode requested for pod %s/%s but CSI driver is not available, falling back to init_container",
				pod.Namespace, pod.Name)
			return NewInitContainerProvider(cfg)
		}
		return NewCSIProvider(cfg)
	}
}

func (f *ProviderFactory) isCSIAvailable() bool {
	// Check if CSI is enabled in the configuration
	if !f.csiEnabled {
		return false
	}

	// Check the CSI driver configMap to see if it is available and SSI-enabled
	csiStatus := GetCSIDriverStatus(context.TODO())
	if !csiStatus.Available {
		return false
	}

	return csiStatus.SSIEnabled
}
