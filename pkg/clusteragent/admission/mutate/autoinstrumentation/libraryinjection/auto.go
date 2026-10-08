// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package libraryinjection

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"golang.org/x/mod/semver"
	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const minCSIKubeVersion = "v1.20"

// libCompat describes which library images ship /datadog-init/package, which
// the CSI driver mounts. Older images only work with init containers.
type libCompat struct {
	// minMajor is the oldest major line with images that ship the layout.
	minMajor string
	// minVersionPerMajor is the first version that ships the layout, per major
	// line. Lines from minMajor that are not listed always ship it. Versions
	// must be X.Y.0 so that minor tags (v2.11) compare correctly.
	minVersionPerMajor map[string]string
}

// minCSILibraryVersions lists, per language, the library images that the CSI
// driver can mount (released in August 2024 or later). Languages that are not
// listed always ship the layout.
var minCSILibraryVersions = map[string]libCompat{
	"java": {
		minMajor: "v1",
		minVersionPerMajor: map[string]string{
			"v1": "v1.38.0",
		},
	},
	"js": {
		minMajor: "v4",
		minVersionPerMajor: map[string]string{
			"v4": "v4.45.0",
			"v5": "v5.21.0",
		},
	},
	"python": {
		minMajor: "v2",
		minVersionPerMajor: map[string]string{
			"v2": "v2.11.0",
		},
	},
	"dotnet": {
		minMajor: "v2",
		minVersionPerMajor: map[string]string{
			"v2": "v2.57.0",
		},
	},
	"ruby": {
		minMajor: "v2",
		minVersionPerMajor: map[string]string{
			"v2": "v2.3.0",
		},
	},
}

// AutoProvider implements LibraryInjectionProvider.
// It picks the best concrete provider for a pod based on the runtime
// environment, currently:
//   - CSIProvider when the Datadog CSI driver is registered in the cluster
//     with APM SSI advertised, the cluster is not OpenShift, runs Kubernetes
//     1.20 or later, all images use supported registries, and all library
//     versions support CSI;
//   - InitContainerProvider otherwise.
type AutoProvider struct {
	realProvider LibraryInjectionProvider
	reason       string
}

// NewAutoProvider creates a new AutoProvider for the given config.
//
// The provider selection is decided at construction time. AutoProvider is
// instantiated per admission request and reads the watcher's cached state
// via a single atomic load, so the hot path stays cheap regardless of how
// often the CSI driver state changes.
func NewAutoProvider(cfg LibraryInjectionConfig) *AutoProvider {
	provider, reason := pickAutoProvider(cfg)
	log.Debugf("library injection auto provider: using %s: %s", provider.GetName(), reason)
	return &AutoProvider{
		realProvider: provider,
		reason:       reason,
	}
}

// pickAutoProvider returns the concrete provider that AutoProvider will
// delegate to, and the reason for the choice.
//
// A nil CSIDriverWatcher always selects the init-container provider.
func pickAutoProvider(cfg LibraryInjectionConfig) (LibraryInjectionProvider, string) {
	if cfg.CSIDriverWatcher == nil || !cfg.CSIDriverWatcher.IsAPMEnabled() {
		return NewInitContainerProvider(cfg), "the CSI driver is not installed or APM is not enabled"
	}
	if cfg.IsOpenShift {
		// Pods injected through the CSI driver need extra privileges on OpenShift.
		return NewInitContainerProvider(cfg), "the cluster runs OpenShift"
	}
	kubeVersion, ok := normalizeKubeSemver(cfg.KubeServerVersion)
	if !ok {
		return NewInitContainerProvider(cfg), "the Kubernetes version is unknown"
	}
	if semver.Compare(semver.MajorMinor(kubeVersion), minCSIKubeVersion) < 0 {
		// Kubelet 1.19 and older create the CSI target path as a directory, so the
		// CSI driver cannot bind-mount the preload file there.
		return NewInitContainerProvider(cfg), fmt.Sprintf("Kubernetes %s is older than %s", kubeVersion, minCSIKubeVersion)
	}
	if registry, found := firstUnsupportedCSIRegistry(cfg); found {
		return NewInitContainerProvider(cfg), fmt.Sprintf("registry %s is not a Datadog public registry", registry)
	}
	for _, library := range cfg.Libraries {
		if !isCSICompatibleLibrary(library) {
			return NewInitContainerProvider(cfg), fmt.Sprintf("%s library version %s predates CSI support", library.Language, libraryVersion(library))
		}
	}
	return NewCSIProvider(cfg), "all CSI requirements are met"
}

func firstUnsupportedCSIRegistry(cfg LibraryInjectionConfig) (string, bool) {
	if !slices.Contains(cfg.CSIAutoRegistries, cfg.Injector.Package.Registry) {
		return cfg.Injector.Package.Registry, true
	}
	for _, library := range cfg.Libraries {
		if !slices.Contains(cfg.CSIAutoRegistries, library.Package.Registry) {
			return library.Package.Registry, true
		}
	}
	return "", false
}

func libraryVersion(library LibraryConfig) string {
	return cmp.Or(library.Package.CanonicalVersion, library.Package.Version)
}

// isCSICompatibleLibrary reports whether the library image ships the layout
// that the CSI driver mounts. Only semver tags are checked: other tags
// (latest, digests, dev builds) are assumed to be recent.
func isCSICompatibleLibrary(library LibraryConfig) bool {
	compat, found := minCSILibraryVersions[library.Language]
	if !found {
		return true
	}
	tag := libraryVersion(library)
	if !strings.HasPrefix(tag, "v") {
		tag = "v" + tag
	}
	if !semver.IsValid(tag) {
		return true
	}
	major := semver.Major(tag)
	if semver.Compare(major, compat.minMajor) < 0 {
		return false
	}
	minVersion, found := compat.minVersionPerMajor[major]
	if !found || tag == major {
		// Newer lines always ship the layout, and major tags (v2) point to
		// the newest release of the line.
		return true
	}
	return semver.Compare(tag, minVersion) >= 0
}

// GetName returns the effective injection mode of the resolved concrete provider,
// suffixed with " (auto)" to indicate that the mode was automatically selected.
func (p *AutoProvider) GetName() string {
	return p.realProvider.GetName() + " (auto)"
}

// Reason explains why the auto mode picked the concrete provider.
func (p *AutoProvider) Reason() string {
	return p.reason
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
