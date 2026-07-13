// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator

package helm

import (
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/Masterminds/semver/v3"
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
// synthetic custom resource. clusterID scopes the release identity to its
// cluster so releases sharing a namespace/name across clusters stay distinct.
func ReleaseToUnstructured(clusterID string, r *Release) *unstructured.Unstructured {
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

	if len(r.History) > 0 {
		spec["history"] = revisionSummariesToInterface(r.History)
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
				"name":            r.Name,
				"namespace":       r.Namespace,
				"uid":             releaseUID(clusterID, r),
				"resourceVersion": r.ResourceVersion,
				"labels": map[string]interface{}{
					"helm_release":       r.Name,
					"helm_revision":      strconv.Itoa(r.Version),
					"helm_status":        status,
					"helm_chart":         chart,
					"helm_chart_version": chartVersion,
					"helm_app_version":   appVersion,
					"helm_last_deployed": lastDeployed,
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

// releaseUID returns a deterministic UUID for a release, stable across revisions
// and unique per cluster
func releaseUID(clusterID string, r *Release) string {
	key := fmt.Sprintf("%s/%s/%s", clusterID, r.Namespace, r.Name)
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(key)).String()
}

// ChartToUnstructured converts a name-aggregated chart into a synthetic custom
// resource: one row per chart name, with the versions seen and how many releases
// use them rolled up. The representative metadata/content is the latest version's.
func ChartToUnstructured(a *ChartAggregate) *unstructured.Unstructured {
	if a == nil || a.Latest == nil || a.Latest.Metadata == nil || a.Latest.Metadata.Name == "" {
		return nil
	}

	c := a.Latest
	name := c.Metadata.Name
	latestVersion := c.Metadata.Version

	spec := map[string]interface{}{
		"name":        name,
		"version":     latestVersion, // the latest version stands in for the chart
		"appVersion":  c.Metadata.AppVersion,
		"apiVersion":  c.Metadata.APIVersion,
		"description": c.Metadata.Description,
		// Rollups so the UI can show one chart summarizing all its versions.
		"versionCount": int64(len(a.Versions)),
		"releaseCount": int64(a.ReleaseCount),
		"versions":     chartVersionsToInterface(a.Versions),
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
				"name":            name,
				"uid":             chartUID(name),
				"resourceVersion": chartResourceVersion(a),
				"labels": map[string]interface{}{
					"helm_chart":         name,
					"helm_chart_version": latestVersion,
					"helm_app_version":   c.Metadata.AppVersion,
					// Surfaced as labels so the counts reach the table as tags.
					"helm_version_count": strconv.Itoa(len(a.Versions)),
					"helm_release_count": strconv.Itoa(a.ReleaseCount),
				},
			},
			"spec": spec,
		},
	}
}

// chartVersionsToInterface converts a chart's per-version rollup into
// JSON-compatible maps (newest first), embedding each version's default values so
// the UI can diff defaults across versions.
func chartVersionsToInterface(versions []ChartVersionSummary) []interface{} {
	out := make([]interface{}, 0, len(versions))
	for _, v := range versions {
		entry := map[string]interface{}{
			"version":    v.Version,
			"appVersion": v.AppVersion,
			"releases":   int64(v.Releases),
		}
		if len(v.DefaultValues) > 0 {
			entry["defaultValues"] = v.DefaultValues
		}
		out = append(out, entry)
	}
	return out
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

// chartUID returns a deterministic UUID for a chart, keyed only by name. A chart
// is a package, not a per-namespace/cluster installation, so its identity is
// intentionally independent of version, namespace, and cluster: the same chart is
// shown once wherever it is used.
func chartUID(name string) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(name)).String()
}

// chartResourceVersion derives a resourceVersion from the aggregate's content so
// the orchestrator re-sends the chart when its versions or release counts change.
func chartResourceVersion(a *ChartAggregate) string {
	h := fnv.New64a()
	for _, v := range a.Versions {
		fmt.Fprintf(h, "%s:%s:%d;", v.Version, v.AppVersion, v.Releases)
	}
	fmt.Fprintf(h, "|%d", a.ReleaseCount)
	return strconv.FormatUint(h.Sum64(), 10)
}

// CurrentReleases returns the latest revision of each release (namespace/name),
// mirroring `helm list`. Helm retains one storage object per revision; we surface
// only the most recent so each release appears once, and attach a summary of all
// revisions as its history.
func CurrentReleases(releases []*Release) []*Release {
	groups := make(map[string][]*Release)
	for _, r := range releases {
		if r == nil {
			continue
		}
		key := r.Namespace + "/" + r.Name
		groups[key] = append(groups[key], r)
	}
	out := make([]*Release, 0, len(groups))
	for _, group := range groups {
		current := group[0]
		for _, r := range group[1:] {
			if r.Version > current.Version {
				current = r
			}
		}
		current.History = revisionSummaries(group)
		out = append(out, current)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Namespace != out[j].Namespace {
			return out[i].Namespace < out[j].Namespace
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// revisionSummaries builds a per-revision summary for a release's revisions,
// newest first.
func revisionSummaries(releases []*Release) []RevisionSummary {
	summaries := make([]RevisionSummary, 0, len(releases))
	for _, r := range releases {
		if r == nil {
			continue
		}
		var chartVersion, appVersion string
		if r.Chart != nil && r.Chart.Metadata != nil {
			chartVersion = r.Chart.Metadata.Version
			appVersion = r.Chart.Metadata.AppVersion
		}
		var status, updated string
		if r.Info != nil {
			status = r.Info.Status
			updated = r.Info.LastDeployed
		}
		summaries = append(summaries, RevisionSummary{
			Revision:     r.Version,
			Status:       status,
			ChartVersion: chartVersion,
			AppVersion:   appVersion,
			Updated:      updated,
			Config:       r.Config,
		})
	}
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].Revision > summaries[j].Revision
	})
	return summaries
}

// revisionSummariesToInterface converts revision summaries into JSON-compatible maps.
func revisionSummariesToInterface(summaries []RevisionSummary) []interface{} {
	out := make([]interface{}, 0, len(summaries))
	for _, s := range summaries {
		entry := map[string]interface{}{
			"revision":     int64(s.Revision),
			"status":       s.Status,
			"chartVersion": s.ChartVersion,
			"appVersion":   s.AppVersion,
			"updated":      s.Updated,
		}
		if len(s.Config) > 0 {
			entry["config"] = s.Config
		}
		out = append(out, entry)
	}
	return out
}

// AggregateCharts collapses the charts referenced by releases into one entry per
// chart name, rolling up the versions seen and how many releases use them.
func AggregateCharts(releases []*Release) []*ChartAggregate {
	groups := make(map[string][]*Release)
	names := make([]string, 0)
	for _, r := range releases {
		if r == nil || r.Chart == nil || r.Chart.Metadata == nil || r.Chart.Metadata.Name == "" {
			continue
		}
		name := r.Chart.Metadata.Name
		if _, ok := groups[name]; !ok {
			names = append(names, name)
		}
		groups[name] = append(groups[name], r)
	}
	sort.Strings(names)

	out := make([]*ChartAggregate, 0, len(names))
	for _, name := range names {
		out = append(out, aggregateChart(groups[name]))
	}
	return out
}

// aggregateChart reduces every release/revision that references a chart name to a
// single ChartAggregate: the latest version's content plus a per-version rollup of
// the distinct releases (namespace/name) that used it.
func aggregateChart(group []*Release) *ChartAggregate {
	type versionAcc struct {
		chart      *Chart
		appVersion string
		releases   map[string]struct{}
	}
	byVersion := make(map[string]*versionAcc)
	order := make([]string, 0)
	releases := make(map[string]struct{})

	for _, r := range group {
		version := r.Chart.Metadata.Version
		acc := byVersion[version]
		if acc == nil {
			acc = &versionAcc{
				chart:      r.Chart,
				appVersion: r.Chart.Metadata.AppVersion,
				releases:   make(map[string]struct{}),
			}
			byVersion[version] = acc
			order = append(order, version)
		}
		releaseKey := r.Namespace + "/" + r.Name
		acc.releases[releaseKey] = struct{}{}
		releases[releaseKey] = struct{}{}
	}

	sortChartVersionsDesc(order)

	versions := make([]ChartVersionSummary, 0, len(order))
	for _, version := range order {
		acc := byVersion[version]
		versions = append(versions, ChartVersionSummary{
			Version:       version,
			AppVersion:    acc.appVersion,
			Releases:      len(acc.releases),
			DefaultValues: acc.chart.Values,
		})
	}

	return &ChartAggregate{
		Latest:       byVersion[order[0]].chart,
		Versions:     versions,
		ReleaseCount: len(releases),
	}
}

// sortChartVersionsDesc orders chart versions newest-first by semver, keeping any
// non-semver versions in reverse-lexical order after the valid ones.
func sortChartVersionsDesc(versions []string) {
	sort.SliceStable(versions, func(i, j int) bool {
		vi, ei := semver.NewVersion(versions[i])
		vj, ej := semver.NewVersion(versions[j])
		switch {
		case ei == nil && ej == nil:
			return vi.GreaterThan(vj)
		case ei == nil:
			return true
		case ej == nil:
			return false
		default:
			return versions[i] > versions[j]
		}
	})
}
