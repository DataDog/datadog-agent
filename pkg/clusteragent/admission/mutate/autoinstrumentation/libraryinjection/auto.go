// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package libraryinjection

import (
	corev1 "k8s.io/api/core/v1"

	wmutil "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	csiAPMEnabledAnnotation = "csi.datadoghq.com/apm-enabled"

	// CSIDriver is a cluster-scoped resource in the storage.k8s.io API group.
	// We look it up via workloadmeta's generic KubernetesMetadata store.
	csiDriverGVRGroup    = "storage.k8s.io"
	csiDriverGVRResource = "csidrivers"
)

// AutoProvider implements LibraryInjectionProvider.
// It picks the best concrete provider for a pod based on the runtime
// environment, currently:
//   - CSIProvider when the Datadog CSI driver is registered in the cluster
//     (detected via workloadmeta);
//   - InitContainerProvider otherwise.
type AutoProvider struct {
	realProvider LibraryInjectionProvider
}

// NewAutoProvider creates a new AutoProvider for the given config.
//
// The provider selection is decided at construction time. AutoProvider is
// instantiated per admission request, so any change in the cluster (e.g. a
// CSI driver being installed) is picked up on the next pod admission without
// requiring a cluster-agent restart.
func NewAutoProvider(cfg LibraryInjectionConfig) *AutoProvider {
	return &AutoProvider{
		realProvider: pickAutoProvider(cfg),
	}
}

// pickAutoProvider returns the concrete provider that AutoProvider will
// delegate to. It is split out from NewAutoProvider for testability.
//
// CSIAutoDetectionEnabled is a temporary feature flag.
func pickAutoProvider(cfg LibraryInjectionConfig) LibraryInjectionProvider {
	if !cfg.CSIAutoDetectionEnabled {
		// CSI auto-detection is opt-in for now; behave like before the feature
		// existed by always returning the init-container provider.
		return NewInitContainerProvider(cfg)
	}
	if isDatadogCSIDriverRegistered(cfg.Wmeta) {
		log.Debugf("library injection auto provider: Datadog CSI driver %q registered, using CSIProvider", csiDriverName)
		return NewCSIProvider(cfg)
	}
	log.Debugf("library injection auto provider: Datadog CSI driver %q not registered, using InitContainerProvider", csiDriverName)
	return NewInitContainerProvider(cfg)
}

// isDatadogCSIDriverRegistered returns true when the Datadog CSI driver is
// known to workloadmeta (i.e. its CSIDriver object exists in the cluster) and
// has explicitly opted into APM SSI volumes via the apm-enabled annotation.
//
// A nil workloadmeta component (in tests or in environments where the
// cluster-agent runs without it) is treated as "not registered", which falls
// back to the safe init-container provider.
func isDatadogCSIDriverRegistered(wmeta workloadmeta.Component) bool {
	if wmeta == nil {
		return false
	}

	id := wmutil.GenerateKubeMetadataEntityID(csiDriverGVRGroup, csiDriverGVRResource, "", csiDriverName)
	driver, err := wmeta.GetKubernetesMetadata(id)
	if err != nil || driver == nil {
		return false
	}

	apmEnabled, ok := driver.Annotations[csiAPMEnabledAnnotation]
	if !ok || apmEnabled != "true" {
		log.Debugf("library injection auto provider: Datadog CSI driver %q is registered but %s != true, falling back to init container",
			csiDriverName, csiAPMEnabledAnnotation)
		return false
	}

	return true
}

// InjectInjector mutates the pod to add the APM injector.
func (p *AutoProvider) InjectInjector(pod *corev1.Pod, cfg InjectorConfig) MutationResult {
	return p.realProvider.InjectInjector(pod, cfg)
}

// InjectLibrary mutates the pod to add a language-specific tracing library.
func (p *AutoProvider) InjectLibrary(pod *corev1.Pod, cfg LibraryConfig) MutationResult {
	return p.realProvider.InjectLibrary(pod, cfg)
}

// Verify that AutoProvider implements LibraryInjectionProvider.
var _ LibraryInjectionProvider = (*AutoProvider)(nil)
