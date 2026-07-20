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
	// History holds a compact summary of every revision of this release. It is
	// computed by the collector (not part of Helm's stored payload) and surfaced
	// on the current release CR so the UI can render release history.
	History []RevisionSummary `json:"-"`
}

// RevisionSummary is a compact per-revision record surfaced on the current
// release CR.
type RevisionSummary struct {
	Revision     int
	Status       string
	ChartVersion string
	AppVersion   string
	// Updated is the revision's last-deployed time.
	Updated string
	// Config holds the user-supplied values for this revision, so the UI can
	// diff values across revisions without re-collecting each one over time.
	Config map[string]interface{}
}

// ChartAggregate collapses every release's chart to one entry per chart name. A
// chart is a package identified by its content, not a per-namespace/cluster
type ChartAggregate struct {
	// Latest is the representative content: the highest version seen.
	Latest *Chart
	// Versions is every distinct version seen, newest first.
	Versions []ChartVersionSummary
	// ReleaseCount is the number of distinct releases using this chart (any version).
	ReleaseCount int
}

// ChartVersionSummary summarizes one version of a chart.
type ChartVersionSummary struct {
	Version    string
	AppVersion string
	// Releases is the number of distinct releases that used this version.
	Releases int
	// DefaultValues holds this version's chart default values, so the UI can diff
	// defaults across versions without re-collecting each one over time.
	DefaultValues map[string]interface{}
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
