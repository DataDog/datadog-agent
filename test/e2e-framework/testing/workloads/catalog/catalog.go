// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package catalog holds the named test workload definitions shared across
// environment types. Each app declares its Kubernetes manifests, its Docker
// image, and its host install command. The environment selects the right
// form; the catalog is data, not provisioning code.
package catalog

import (
	"fmt"
	"sort"
	"strings"
)

// App is one named test workload in the catalog.
type App struct {
	Name        string
	Description string
	// Image is the Docker image (for Docker-capable environments).
	Image string
	// Ports are the container ports that should be exposed.
	Ports []int
	// K8sNamespace is the namespace for Kubernetes deployments (if the
	// standard pattern applies).
	K8sNamespace string
}

// Catalog is the explicit registry of named test workloads. Adding a new
// workload means adding one entry here plus the K8s manifest if needed.
var Catalog = map[string]App{
	"nginx": {
		Name:         "nginx",
		Description:  "NGINX web server for AD-annotation and HTTP check testing",
		Image:        "ghcr.io/datadog/apps-nginx-server:latest",
		Ports:        []int{80},
		K8sNamespace: "workload-nginx",
	},
	"redis": {
		Name:         "redis",
		Description:  "Redis for auto-discovery and endpoint check testing",
		Image:        "ghcr.io/datadog/redis:latest",
		Ports:        []int{6379},
		K8sNamespace: "workload-redis",
	},
	"cpustress": {
		Name:         "cpustress",
		Description:  "stress-ng for CPU metrics with resource limits",
		Image:        "ghcr.io/datadog/apps-stress-ng:latest",
		K8sNamespace: "workload-cpustress",
	},
	"tracegen": {
		Name:         "tracegen",
		Description:  "Trace generator for APM testing",
		Image:        "ghcr.io/datadog/apps-tracegen:latest",
		K8sNamespace: "workload-tracegen",
	},
	"dogstatsd": {
		Name:         "dogstatsd",
		Description:  "Dogstatsd client for custom metrics testing",
		Image:        "ghcr.io/datadog/apps-dogstatsd:latest",
		K8sNamespace: "workload-dogstatsd",
	},
}

// Get returns the catalog entry for an app on a specific base, if available.
func Get(name, base string) (App, bool) {
	app, ok := Catalog[name]
	if !ok {
		return App{}, false
	}
	switch base {
	case "kind", "eks":
		// Kubernetes environments use K8s manifests; the image is available
		// as a fallback for bare-image deployments.
		return app, true
	case "local":
		// The local container agent runs Docker containers; only the image
		// is relevant.
		return app, app.Image != ""
	case "ec2-host":
		// Host environments can run Docker containers via SSH or install
		// packages; the image is the simplest path.
		return app, app.Image != ""
	}
	return App{}, false
}

// Has reports whether an app has a definition for the given base.
func Has(name, base string) bool {
	_, ok := Get(name, base)
	return ok
}

// AvailableForms returns which forms an app supports on the given base.
// Used in validation error messages.
func AvailableForms(name string) []string {
	app, ok := Catalog[name]
	if !ok {
		return []string{"unknown"}
	}
	forms := []string{}
	if app.Image != "" {
		forms = append(forms, "image")
	}
	if app.K8sNamespace != "" {
		forms = append(forms, "k8s-manifest")
	}
	return forms
}

// Manifests returns the Kubernetes manifests for an app on a Kubernetes
// base. This is a simple Deployment + Service for each catalog entry;
// the full AD-annotation manifests from the containers test suite can be
// layered in later without changing the catalog interface.
func Manifests(name, base, namespace string) ([]string, error) {
	app, ok := Get(name, base)
	if !ok {
		return nil, fmt.Errorf("app %q is not available on base %q", name, base)
	}
	if app.K8sNamespace == "" {
		return nil, fmt.Errorf("app %q has no Kubernetes definition", name)
	}
	if namespace == "" {
		namespace = app.K8sNamespace
	}
	manifest := fmt.Sprintf(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: %s
  namespace: %s
  labels:
    app: %s
spec:
  replicas: 1
  selector:
    matchLabels:
      app: %s
  template:
    metadata:
      labels:
        app: %s
      annotations:
        ad.datadoghq.com/%s.checks: |
          {
            "nginx": {
              "init_config": {},
              "instances": [
                {
                  "nginx_status_url": "http://%%%%host%%%%/nginx_status/"
                }
              ]
            }
          }
    spec:
      containers:
        - name: %s
          image: %s
          ports:
            - containerPort: 80
---
apiVersion: v1
kind: Service
metadata:
  name: %s
  namespace: %s
spec:
  selector:
    app: %s
  ports:
    - port: 80
      targetPort: 80
`, name, namespace, name, name, name, name, name, app.Image, name, namespace, name)
	return []string{manifest}, nil
}

// Names returns the sorted catalog names for error messages.
func Names() []string {
	names := make([]string, 0, len(Catalog))
	for name := range Catalog {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Summary returns a human-readable summary of the catalog.
func Summary() string {
	var sb strings.Builder
	for _, name := range Names() {
		app := Catalog[name]
		sb.WriteString(fmt.Sprintf("  %-12s %s\n", name, app.Description))
	}
	return sb.String()
}
