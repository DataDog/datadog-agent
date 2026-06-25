// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator

package helm

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// HelmReleaseGroup is the synthetic API group used to surface Helm releases as
// custom resources in the orchestrator.
const HelmReleaseGroup = "helm.datadoghq.com"

// HelmReleaseVersion is the synthetic API version for the HelmRelease resource.
const HelmReleaseVersion = "v1"

// HelmReleaseKind is the synthetic Kind for the HelmRelease resource.
const HelmReleaseKind = "HelmRelease"

// HelmChartKind is the synthetic Kind for packaged Helm charts.
const HelmChartKind = "HelmChart"

const helmReleaseAPIVersion = HelmReleaseGroup + "/" + HelmReleaseVersion

// ReleaseToUnstructured converts a parsed Helm release revision into a
// synthetic custom resource.
func ReleaseToUnstructured(r *Release) *unstructured.Unstructured {
	var chart, chartVersion, appVersion string
	if r.Chart != nil && r.Chart.Metadata != nil {
		chart = r.Chart.Metadata.Name
		chartVersion = r.Chart.Metadata.Version
		appVersion = r.Chart.Metadata.AppVersion
	}

	var status, firstDeployed, lastDeployed string
	if r.Info != nil {
		status = r.Info.Status
		firstDeployed = r.Info.FirstDeployed
		lastDeployed = r.Info.LastDeployed
	}

	spec := map[string]interface{}{
		"releaseName":   r.Name,
		"revision":      int64(r.Version),
		"chart":         chart,
		"chartVersion":  chartVersion,
		"appVersion":    appVersion,
		"status":        status,
		"firstDeployed": firstDeployed,
		"lastDeployed":  lastDeployed,
		// Link to the chart object so chart data is not duplicated per release.
		"chartRef": map[string]interface{}{
			"name":    chart,
			"version": chartVersion,
		},
	}

	if len(r.Config) > 0 {
		spec["config"] = r.Config
	}

	// Keep manifests structured so the scrubber can inspect nested fields.
	if resources := parseManifest(r.Manifest); len(resources) > 0 {
		spec["resources"] = resources
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": helmReleaseAPIVersion,
			"kind":       HelmReleaseKind,
			"metadata": map[string]interface{}{
				"name":            fmt.Sprintf("%s.v%d", r.Name, r.Version),
				"namespace":       r.Namespace,
				"uid":             releaseUID(r),
				"resourceVersion": "1", // a given revision is immutable once stored
				"labels": map[string]interface{}{
					"helm_release":  r.Name,
					"helm_revision": strconv.Itoa(r.Version),
					"helm_status":   status,
				},
			},
			"spec": spec,
		},
	}
}

// templatesToInterface converts chart templates into JSON-compatible maps.
func templatesToInterface(templates []*Template) []interface{} {
	out := make([]interface{}, 0, len(templates))
	for _, t := range templates {
		if t == nil {
			continue
		}
		out = append(out, map[string]interface{}{
			"name": t.Name,
			"data": string(t.Data),
		})
	}
	return out
}

// parseManifest splits a rendered multi-document YAML manifest.
func parseManifest(manifest string) []interface{} {
	if strings.TrimSpace(manifest) == "" {
		return nil
	}

	var resources []interface{}
	decoder := utilyaml.NewYAMLOrJSONDecoder(strings.NewReader(manifest), 4096)
	for {
		obj := map[string]interface{}{}
		if err := decoder.Decode(&obj); err != nil {
			if !errors.Is(err, io.EOF) {
				log.Debugf("helm: stopping manifest parse after a malformed document: %v", err)
			}
			break
		}
		if len(obj) == 0 {
			continue
		}
		resources = append(resources, obj)
	}
	return resources
}

// releaseUID returns a deterministic UUID for a release revision.
func releaseUID(r *Release) string {
	key := fmt.Sprintf("%s/%s/%d", r.Namespace, r.Name, r.Version)
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(key)).String()
}

// ChartToUnstructured converts a packaged chart into a synthetic custom resource.
func ChartToUnstructured(c *Chart) *unstructured.Unstructured {
	if c == nil || c.Metadata == nil || c.Metadata.Name == "" {
		return nil
	}

	name := c.Metadata.Name
	version := c.Metadata.Version

	spec := map[string]interface{}{
		"name":        name,
		"version":     version,
		"appVersion":  c.Metadata.AppVersion,
		"apiVersion":  c.Metadata.APIVersion,
		"description": c.Metadata.Description,
	}

	if len(c.Values) > 0 {
		spec["defaultValues"] = c.Values
	}
	if len(c.Templates) > 0 {
		spec["templates"] = templatesToInterface(c.Templates)
	}
	if deps := dependenciesToInterface(c.Metadata.Dependencies); len(deps) > 0 {
		spec["dependencies"] = deps
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": helmReleaseAPIVersion,
			"kind":       HelmChartKind,
			"metadata": map[string]interface{}{
				"name":            fmt.Sprintf("%s.%s", name, version),
				"uid":             chartUID(name, version),
				"resourceVersion": "1", // chart content for a version is immutable
				"labels": map[string]interface{}{
					"helm_chart":         name,
					"helm_chart_version": version,
				},
			},
			"spec": spec,
		},
	}
}

// dependenciesToInterface converts chart dependencies into JSON-compatible maps.
func dependenciesToInterface(deps []*Dependency) []interface{} {
	out := make([]interface{}, 0, len(deps))
	for _, d := range deps {
		if d == nil {
			continue
		}
		out = append(out, map[string]interface{}{
			"name":       d.Name,
			"version":    d.Version,
			"repository": d.Repository,
			"condition":  d.Condition,
			"enabled":    d.Enabled,
			"alias":      d.Alias,
		})
	}
	return out
}

func chartUID(name, version string) string {
	key := fmt.Sprintf("%s/%s", name, version)
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(key)).String()
}

// UniqueCharts returns one chart per distinct (name, version).
func UniqueCharts(releases []*Release) []*Chart {
	seen := make(map[string]struct{})
	charts := make([]*Chart, 0)
	for _, r := range releases {
		if r == nil || r.Chart == nil || r.Chart.Metadata == nil || r.Chart.Metadata.Name == "" {
			continue
		}
		key := r.Chart.Metadata.Name + "\x00" + r.Chart.Metadata.Version
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		charts = append(charts, r.Chart)
	}
	return charts
}
