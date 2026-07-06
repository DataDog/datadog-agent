// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator

// Package helm decodes Helm 3 releases stored in the cluster into structured Go
// objects so the orchestrator can collect Helm release and chart information.
package helm

// These structs mirror the Helm release payload without importing Helm.
// Ref: https://github.com/helm/helm/blob/v3.8.0/pkg/release/release.go#L22

// Release represents a single Helm release revision.
type Release struct {
	Name      string `json:"name,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	// Version is the release revision number. It is incremented on every
	// "helm upgrade", and together with Namespace and Name uniquely identifies
	// a storage object.
	Version int    `json:"version,omitempty"`
	Info    *Info  `json:"info,omitempty"`
	Chart   *Chart `json:"chart,omitempty"`
	// Config holds the user-supplied values that override the chart defaults.
	Config map[string]interface{} `json:"config,omitempty"`
	// Manifest is the fully rendered Kubernetes YAML applied to the cluster.
	Manifest string `json:"manifest,omitempty"`
	// ResourceVersion is the backing storage object's resourceVersion.
	ResourceVersion string `json:"-"`
}

// Info describes the deployment state of a release.
type Info struct {
	FirstDeployed string `json:"first_deployed,omitempty"`
	LastDeployed  string `json:"last_deployed,omitempty"`
	Deleted       string `json:"deleted,omitempty"`
	Description   string `json:"description,omitempty"`
	// Status is the release status, for example "deployed", "superseded" or
	// "failed".
	Status string `json:"status,omitempty"`
	Notes  string `json:"notes,omitempty"`
}

// Chart holds the chart packaged with a release.
type Chart struct {
	Metadata *Metadata `json:"metadata,omitempty"`
	// Values are the default configuration values for the chart.
	Values map[string]interface{} `json:"values,omitempty"`
	// Templates are the chart's template files, before rendering.
	Templates []*Template `json:"templates,omitempty"`
}

// Metadata holds the chart's identifying information.
type Metadata struct {
	Name         string        `json:"name,omitempty"`
	Version      string        `json:"version,omitempty"`
	AppVersion   string        `json:"appVersion,omitempty"`
	Description  string        `json:"description,omitempty"`
	APIVersion   string        `json:"apiVersion,omitempty"`
	Dependencies []*Dependency `json:"dependencies,omitempty"`
}

// Dependency describes a chart that the parent chart depends on (a subchart).
type Dependency struct {
	Name       string `json:"name,omitempty"`
	Version    string `json:"version,omitempty"`
	Repository string `json:"repository,omitempty"`
	// Condition is a values path (for example "datadog.operator.enabled") that
	// toggles whether the dependency is included.
	Condition string `json:"condition,omitempty"`
	Enabled   bool   `json:"enabled,omitempty"`
	Alias     string `json:"alias,omitempty"`
}

// Template is a single chart template file. Data holds the raw file contents;
// encoding/json transparently base64-decodes it during unmarshalling.
type Template struct {
	Name string `json:"name,omitempty"`
	Data []byte `json:"data,omitempty"`
}
