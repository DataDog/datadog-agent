// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package catalog defines the named test workloads available through the
// e2ectl `workloads:` config section. Each entry is a data-only description:
// the Docker image for container-native environments and the Kubernetes
// manifests for cluster environments.
//
// The Kubernetes manifests are ported from the framework's Pulumi app
// definitions (components/datadog/apps/...) so the original test suites
// (e.g. the containers k8sSuite) see the same deployments, namespaces,
// labels and annotations they assert against. Deployment names, container
// names, images and tag-bearing labels must stay in sync with those
// definitions; the containers tests assert them exactly.
//
// Manifests may carry template variables substituted by the deployer:
//
//	{{API_KEY}}         the agent API key (runner profile)
//	{{FAKEINTAKE_URL}}  the environment's fakeintake URL
//	{{CLUSTER_NAME}}    the environment (cluster) name
package catalog

import (
	"fmt"
	"strings"
)

// Version matches components/datadog/apps.Version — the tag of every
// test-infra-definitions app image the suites assert against.
const Version = "v0.0.7"

// App describes one named workload.
type App struct {
	Name        string
	Description string
	// Image is the Docker image (for Docker-capable environments).
	Image string
	// Ports the image exposes.
	Ports []int
	// K8sNamespace is the namespace for Kubernetes deployments.
	K8sNamespace string
	// k8sManifests renders the Kubernetes manifests (multi-document YAML)
	// into the given namespace. Manifests are validated by the tests that
	// consume them; only apply-time errors surface here.
	k8sManifests func(namespace string) []string
	// k8sWaitNames are the Deployments the deployer waits on. Empty means
	// no wait: cluster-scoped definitions (CRDs), DaemonSets, or apps
	// whose readiness the tests themselves assert.
	k8sWaitNames []string
}

// K8sWaitNames returns the Deployments to wait on for the named app.
func K8sWaitNames(name string) []string {
	if app, ok := apps[name]; ok {
		return app.k8sWaitNames
	}
	return nil
}

var apps = map[string]App{
	"nginx": {
		Name:         "nginx",
		Description:  "NGINX web server for AD-annotation, HTTP check and endpoint testing (parity with apps/nginx)",
		Image:        "ghcr.io/datadog/apps-nginx-server:" + Version,
		Ports:        []int{80},
		K8sNamespace: "workload-nginx",
		k8sManifests: nginxManifests,
		k8sWaitNames: []string{"nginx"},
	},
	"redis": {
		Name:         "redis",
		Description:  "Redis for auto-discovery and endpoint check testing (parity with apps/redis)",
		Image:        "ghcr.io/datadog/redis:" + Version,
		Ports:        []int{6379},
		K8sNamespace: "workload-redis",
		k8sManifests: redisManifests,
		k8sWaitNames: []string{"redis"},
	},
	"cpustress": {
		Name:         "cpustress",
		Description:  "CPU workload for container CPU metrics testing (parity with apps/cpustress)",
		Image:        "ghcr.io/datadog/apps-stress-ng:" + Version,
		K8sNamespace: "workload-cpustress",
		k8sManifests: cpustressManifests,
		k8sWaitNames: []string{"stress-ng"},
	},
	"dogstatsd": {
		Name:         "dogstatsd",
		Description:  "DogStatsD clients over UDP, UDS and CSI (parity with apps/dogstatsd)",
		Image:        "ghcr.io/datadog/apps-dogstatsd:" + Version,
		K8sNamespace: "workload-dogstatsd",
		k8sManifests: dogstatsdManifests,
		k8sWaitNames: []string{"dogstatsd-uds-with-csi", "dogstatsd-uds", "dogstatsd-udp", "dogstatsd-udp-origin-detection", "dogstatsd-udp-contname-injected", "dogstatsd-udp-external-data-only"},
	},
	"dogstatsd-standalone": {
		Name:         "dogstatsd-standalone",
		Description:  "Standalone DogStatsD daemonset receiving metrics on the host port (parity with dogstatsd-standalone)",
		Image:        "registry.datadoghq.com/dogstatsd:latest",
		K8sNamespace: "dogstatsd-standalone",
		k8sManifests: dogstatsdStandaloneManifests,
	},
	"dogstatsd-standalone-clients": {
		Name:         "dogstatsd-standalone-clients",
		Description:  "DogStatsD clients reporting to the standalone daemonset (deploy after dogstatsd-standalone)",
		Image:        "ghcr.io/datadog/apps-dogstatsd:" + Version,
		K8sNamespace: "workload-dogstatsd-standalone",
		k8sManifests: dogstatsdStandaloneClientsManifests,
		k8sWaitNames: []string{"dogstatsd-uds", "dogstatsd-udp"},
	},
	"tracegen": {
		Name:         "tracegen",
		Description:  "Trace generator over TCP and UDS (parity with apps/tracegen)",
		Image:        "ghcr.io/datadog/apps-tracegen:" + Version,
		K8sNamespace: "workload-tracegen",
		k8sManifests: tracegenManifests,
		k8sWaitNames: []string{"tracegen-tcp", "tracegen-uds"},
	},
	"prometheus": {
		Name:         "prometheus",
		Description:  "Prometheus endpoint for the openmetrics check (parity with apps/prometheus)",
		Image:        "ghcr.io/datadog/apps-prometheus:" + Version,
		Ports:        []int{8080},
		K8sNamespace: "workload-prometheus",
		k8sManifests: prometheusManifests,
		k8sWaitNames: []string{"prometheus"},
	},
	"etcd": {
		Name:         "etcd",
		Description:  "etcd v2 server carrying an AD check config in keys (parity with apps/etcd)",
		Image:        "quay.io/coreos/etcd:v3.5.1",
		Ports:        []int{2379},
		K8sNamespace: "etcd",
		k8sManifests: etcdManifests,
		k8sWaitNames: []string{"etcd"},
	},
	"vpa-crd": {
		Name:         "vpa-crd",
		Description:  "The minimal VerticalPodAutoscaler CRD so KSM can report VPA metrics (parity with components/kubernetes/vpa)",
		K8sNamespace: "",
		k8sManifests: vpaCrdManifests,
	},
	"mutated": {
		Name:         "mutated",
		Description:  "Workloads mutated by the admission controller, with and without library injection (parity with apps/mutatedbyadmissioncontroller)",
		Image:        "ghcr.io/datadog/apps-mutated:" + Version,
		K8sNamespace: "workload-mutated",
		k8sManifests: mutatedManifests,
		k8sWaitNames: []string{
			"mutated",
			// These two live in the lib-injection namespace — names may
			// carry an explicit namespace prefix.
			"workload-mutated-lib-injection/mutated-with-lib-annotation",
			"workload-mutated-lib-injection/mutated-with-auto-detected-language",
		},
	},
}

// Get returns the catalog entry for name and reports whether it has a
// definition for the given environment base.
func Get(name, base string) (App, bool) {
	app, ok := apps[name]
	if !ok {
		return App{}, false
	}
	switch base {
	case "kind", "eks", "local":
		return app, true
	default:
		return App{}, false
	}
}

// Has reports whether the named app exists at all.
func Has(name string) bool {
	_, ok := apps[name]
	return ok
}

// AvailableForms returns which deployment forms the named app supports.
func AvailableForms(name string) []string {
	if Has(name) {
		return []string{"app"}
	}
	return nil
}

// Names lists catalog entries.
func Names() []string { return sortedNames() }

// Summary returns a one-line listing for error messages and help output.
func Summary() string {
	var b strings.Builder
	for _, n := range sortedNames() {
		app := apps[n]
		fmt.Fprintf(&b, "  %-30s %s\n", n, app.Description)
	}
	return b.String()
}

func sortedNames() []string {
	order := []string{"vpa-crd", "nginx", "redis", "cpustress", "dogstatsd", "dogstatsd-standalone", "dogstatsd-standalone-clients", "tracegen", "prometheus", "etcd", "mutated"}
	out := make([]string, 0, len(order))
	for _, n := range order {
		if _, ok := apps[n]; ok {
			out = append(out, n)
		}
	}
	return out
}

// Manifests renders the Kubernetes manifests for the named app into
// namespace (the entry's default when empty). The returned documents are
// multi-document YAML strings.
func Manifests(name, base, namespace string) ([]string, error) {
	app, ok := Get(name, base)
	if !ok {
		return nil, fmt.Errorf("app %q is not available on base %q", name, base)
	}
	if app.k8sManifests == nil {
		return nil, fmt.Errorf("app %q has no Kubernetes definition", name)
	}
	if namespace == "" {
		namespace = app.K8sNamespace
	}
	return app.k8sManifests(namespace), nil
}
