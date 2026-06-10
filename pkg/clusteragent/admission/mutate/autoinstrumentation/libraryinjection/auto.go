// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package libraryinjection

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// AutoProvider implements LibraryInjectionProvider.
// It picks the best concrete provider for a pod based on the runtime
// environment, currently:
//   - CSIProvider when the Datadog CSI driver is registered in the cluster
//     with APM SSI advertised (state cached by CSIDriverWatcher);
//   - InitContainerProvider otherwise.
type AutoProvider struct {
	realProvider LibraryInjectionProvider
}

// NewAutoProvider creates a new AutoProvider for the given config.
//
// The provider selection is decided at construction time. AutoProvider is
// instantiated per admission request and reads the watcher's cached state
// via a single atomic load, so the hot path stays cheap regardless of how
// often the CSI driver state changes.
func NewAutoProvider(cfg LibraryInjectionConfig) *AutoProvider {
	return &AutoProvider{
		realProvider: pickAutoProvider(cfg),
	}
}

// pickAutoProvider returns the concrete provider that AutoProvider will
// delegate to. It is split out from NewAutoProvider for testability.
//
// A nil CSIDriverWatcher disables CSI auto-detection — for example when the
// temporary feature flag apm_config.instrumentation.csi_driver_detection_enabled
// is off, or in unit tests that don't care about CSI selection. In that case
// the provider behaves exactly as before this feature existed and always
// returns the init-container provider.
func pickAutoProvider(cfg LibraryInjectionConfig) LibraryInjectionProvider {
	if cfg.CSIDriverWatcher == nil {
		return NewInitContainerProvider(cfg)
	}
	if cfg.CSIDriverWatcher.IsAPMEnabled() {
		log.Debugf("library injection auto provider: Datadog CSI driver %q is registered with APM enabled, using CSIProvider", csiDriverName)
		return NewCSIProvider(cfg)
	}
	log.Debugf("library injection auto provider: Datadog CSI driver %q is not registered (or APM not advertised), using InitContainerProvider", csiDriverName)
	return NewInitContainerProvider(cfg)
}

// GetName returns the effective injection mode of the resolved concrete provider,
// suffixed with " (auto)" to indicate that the mode was automatically selected.
func (p *AutoProvider) GetName() string {
	return p.realProvider.GetName() + " (auto)"
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
