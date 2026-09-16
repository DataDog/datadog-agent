// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package main validates emitted GPU metrics against the shared spec.
package main

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"path"
	"slices"
	"sort"
	"strings"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	datadogV2 "github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"

	gpuspec "github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/spec"
)

type metricsClient struct {
	api *datadogV2.MetricsApi
	ctx context.Context
}

const maxAPIErrorBodyLength = 4 * 1024

type apiErrorWithBody interface {
	Body() []byte
}

func includeAPIErrorBody(err error) error {
	var apiErr apiErrorWithBody
	if !errors.As(err, &apiErr) {
		return err
	}

	body := strings.TrimSpace(string(apiErr.Body()))
	if body == "" {
		return err
	}
	if len(body) > maxAPIErrorBodyLength {
		body = body[:maxAPIErrorBodyLength] + "... (truncated)"
	}

	return fmt.Errorf("%w: API response: %s", err, body)
}

func newMetricsClient(apiKey, appKey, site string) (*metricsClient, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("api key is required")
	}
	if strings.TrimSpace(appKey) == "" {
		return nil, errors.New("app key is required")
	}
	if strings.TrimSpace(site) == "" {
		return nil, errors.New("site is required")
	}

	ctx := context.WithValue(
		context.Background(),
		datadog.ContextAPIKeys,
		map[string]datadog.APIKey{
			"apiKeyAuth": {Key: apiKey},
			"appKeyAuth": {Key: appKey},
		},
	)
	ctx = context.WithValue(ctx, datadog.ContextServerVariables, map[string]string{"site": site})

	return &metricsClient{
		api: datadogV2.NewMetricsApi(datadog.NewAPIClient(datadog.NewConfiguration())),
		ctx: ctx,
	}, nil
}

func buildScalarQuery(name, query string, aggregator datadogV2.MetricsAggregator) datadogV2.ScalarQuery {
	q := datadogV2.NewMetricsScalarQuery(aggregator, datadogV2.METRICSDATASOURCE_METRICS, query)
	q.SetName(name)
	return datadogV2.MetricsScalarQueryAsScalarQuery(q)
}

type scalarResult struct {
	tags   map[string]string
	values map[string]*float64
}

type agentVersionFilter struct {
	metricFilter string
	tagFilters   []string
}

var agentFilterCandidateTags = []string{
	"datacenter",
	"region",
	"cloud_provider",
	"kube_cluster_name",
}

type clusterAgentMetadata struct {
	versions map[string]struct{}
	tags     map[string]map[string]struct{}
}

type tagFilterCandidate struct {
	filter string
	cover  map[string]struct{}
}

func (c *metricsClient) runScalarQueries(queries []datadogV2.ScalarQuery, fromTS, toTS int64) ([]scalarResult, error) {
	attrs := datadogV2.NewScalarFormulaRequestAttributes(fromTS*1000, queries, toTS*1000)
	req := datadogV2.NewScalarFormulaRequest(*attrs, datadogV2.SCALARFORMULAREQUESTTYPE_SCALAR_REQUEST)
	body := datadogV2.NewScalarFormulaQueryRequest(*req)

	response, httpResp, err := c.api.QueryScalarData(c.ctx, *body)
	if httpResp != nil && httpResp.Body != nil {
		_ = httpResp.Body.Close()
	}
	if err != nil {
		return nil, fmt.Errorf("query scalar data: %w", includeAPIErrorBody(err))
	}
	if response.Errors != nil && strings.TrimSpace(*response.Errors) != "" {
		return nil, fmt.Errorf("query scalar data returned errors: %s", strings.TrimSpace(*response.Errors))
	}
	if response.Data == nil || response.Data.Attributes == nil {
		return nil, errors.New("query scalar data returned no data")
	}

	return splitScalarColumns(response.Data.Attributes.Columns)
}

func splitScalarColumns(columns []datadogV2.ScalarColumn) ([]scalarResult, error) {
	var dataCols []*datadogV2.DataScalarColumn
	var groupCols []*datadogV2.GroupScalarColumn
	numResults := 0

	for _, column := range columns {
		if column.DataScalarColumn != nil {
			dataCols = append(dataCols, column.DataScalarColumn)
			numResults = max(numResults, len(column.DataScalarColumn.GetValues()))
		} else if column.GroupScalarColumn != nil {
			groupCols = append(groupCols, column.GroupScalarColumn)
			numResults = max(numResults, len(column.GroupScalarColumn.GetValues()))
		}
	}

	results := make([]scalarResult, numResults)
	for idx := range results {
		results[idx] = scalarResult{
			tags:   map[string]string{},
			values: map[string]*float64{},
		}
	}

	for _, column := range dataCols {
		for idx, value := range column.GetValues() {
			if idx >= numResults {
				return nil, fmt.Errorf("query scalar data returned unexpected number of results, expected %d but got %d", numResults, idx+1)
			}

			if value != nil {
				results[idx].values[column.GetName()] = value
			}
		}
	}

	for _, column := range groupCols {
		for idx, tags := range column.GetValues() {
			if idx >= numResults {
				return nil, fmt.Errorf("query scalar data returned unexpected number of results, expected %d but got %d", numResults, idx+1)
			}

			if tags != nil {
				results[idx].tags[column.GetName()] = strings.Join(tags, ",")
			}
		}
	}

	return results, nil
}

func (c *metricsClient) queryDeviceCount(config gpuspec.GPUConfig, queryFilter string, fromTS, toTS int64) (int, error) {
	columns, err := c.runScalarQueries(
		[]datadogV2.ScalarQuery{
			buildScalarQuery("q0", fmt.Sprintf("avg:gpu.device.total{%s} by {gpu_uuid}", queryFilter), datadogV2.METRICSAGGREGATOR_AVG),
		},
		fromTS,
		toTS,
	)
	if err != nil {
		return 0, fmt.Errorf("query device count for %s/%s: %w", config.Architecture, config.DeviceMode, err)
	}

	return len(columns), nil
}

func (c *metricsClient) filterForAgentVersion(agentVersion string, fromTS, toTS int64) (agentVersionFilter, error) {
	columns, err := c.runScalarQueries(
		[]datadogV2.ScalarQuery{
			buildScalarQuery(
				"q0",
				"avg:datadog.agent.running{*} by {kube_cluster_name,image_tag}",
				datadogV2.METRICSAGGREGATOR_AVG,
			),
		},
		fromTS,
		toTS,
	)
	if err != nil {
		return agentVersionFilter{}, fmt.Errorf("query agent versions by Kubernetes cluster: %w", err)
	}

	metadataByCluster := make(map[string]*clusterAgentMetadata)
	for _, column := range columns {
		cluster := column.tags["kube_cluster_name"]
		if isNullishGroupValue(cluster) {
			continue
		}
		metadata := ensureClusterAgentMetadata(metadataByCluster, cluster)
		if imageTag := column.tags["image_tag"]; !isNullishGroupValue(imageTag) {
			metadata.versions[imageTag] = struct{}{}
		}
		metadata.tags["kube_cluster_name"] = map[string]struct{}{cluster: {}}
	}

	for _, tagName := range agentFilterCandidateTags {
		if tagName == "kube_cluster_name" {
			continue
		}
		tagColumns, err := c.runScalarQueries(
			[]datadogV2.ScalarQuery{
				buildScalarQuery(
					"q0",
					fmt.Sprintf("avg:datadog.agent.running{*} by {kube_cluster_name,%s}", tagName),
					datadogV2.METRICSAGGREGATOR_AVG,
				),
			},
			fromTS,
			toTS,
		)
		if err != nil {
			return agentVersionFilter{}, fmt.Errorf("query agent %s tags by Kubernetes cluster: %w", tagName, err)
		}
		for _, column := range tagColumns {
			cluster := column.tags["kube_cluster_name"]
			tagValue := column.tags[tagName]
			if isNullishGroupValue(cluster) || isNullishGroupValue(tagValue) {
				continue
			}
			metadata := ensureClusterAgentMetadata(metadataByCluster, cluster)
			if metadata.tags[tagName] == nil {
				metadata.tags[tagName] = make(map[string]struct{})
			}
			metadata.tags[tagName][tagValue] = struct{}{}
		}
	}

	clusters := make([]string, 0, len(metadataByCluster))
	for cluster, metadata := range metadataByCluster {
		if len(metadata.versions) == 0 {
			continue
		}
		matchesVersion := true
		for version := range metadata.versions {
			matches, err := path.Match(agentVersion, version)
			if err != nil {
				return agentVersionFilter{}, fmt.Errorf("match agent version %q: %w", agentVersion, err)
			}
			if !matches {
				matchesVersion = false
				break
			}
		}
		if matchesVersion {
			clusters = append(clusters, cluster)
		}
	}
	if len(clusters) == 0 {
		return agentVersionFilter{}, fmt.Errorf("no Kubernetes clusters exclusively run agent version %q", agentVersion)
	}

	sort.Strings(clusters)
	return agentVersionFilter{
		metricFilter: fmt.Sprintf("kube_cluster_name:(%s)", strings.Join(clusters, " OR ")),
		tagFilters:   minimumTagFiltersForClusters(metadataByCluster, clusters),
	}, nil
}

func ensureClusterAgentMetadata(metadataByCluster map[string]*clusterAgentMetadata, cluster string) *clusterAgentMetadata {
	if metadataByCluster[cluster] == nil {
		metadataByCluster[cluster] = &clusterAgentMetadata{
			versions: make(map[string]struct{}),
			tags:     make(map[string]map[string]struct{}),
		}
	}
	return metadataByCluster[cluster]
}

func minimumTagFiltersForClusters(metadataByCluster map[string]*clusterAgentMetadata, targetClusters []string) []string {
	targetClusterSet := make(map[string]struct{}, len(targetClusters))
	for _, cluster := range targetClusters {
		targetClusterSet[cluster] = struct{}{}
	}

	candidatesByFilter := make(map[string]map[string]struct{})
	unsafeCandidates := make(map[string]struct{})
	for cluster, metadata := range metadataByCluster {
		_, isTarget := targetClusterSet[cluster]
		for tagName, tagValues := range metadata.tags {
			for tagValue := range tagValues {
				filter := tagName + ":" + tagValue
				if !isTarget {
					// This candidate would include a cluster that does not run
					// the requested Agent version.
					unsafeCandidates[filter] = struct{}{}
					continue
				}
				if candidatesByFilter[filter] == nil {
					candidatesByFilter[filter] = make(map[string]struct{})
				}
				candidatesByFilter[filter][cluster] = struct{}{}
			}
		}
	}

	candidates := make([]tagFilterCandidate, 0, len(candidatesByFilter))
	for filter, cover := range candidatesByFilter {
		if _, unsafe := unsafeCandidates[filter]; !unsafe {
			candidates = append(candidates, tagFilterCandidate{filter: filter, cover: cover})
		}
	}
	selected := minimumTagFilterCover(candidates, targetClusterSet)
	if len(selected) == 0 {
		return clusterTagFilters(targetClusters)
	}
	return selected
}

func clusterTagFilters(clusters []string) []string {
	filters := make([]string, 0, len(clusters))
	for _, cluster := range clusters {
		filters = append(filters, "kube_cluster_name:"+cluster)
	}
	return filters
}

func minimumTagFilterCover(candidates []tagFilterCandidate, targetClusters map[string]struct{}) []string {
	// A candidate contained by another has no advantage when every filter has the
	// same cost. Discarding it substantially reduces the selection space.
	slices.SortFunc(candidates, func(a, b tagFilterCandidate) int {
		if countDiff := len(b.cover) - len(a.cover); countDiff != 0 {
			return countDiff
		}
		return strings.Compare(a.filter, b.filter)
	})
	filtered := make([]tagFilterCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if slices.ContainsFunc(filtered, func(other tagFilterCandidate) bool {
			return candidateCoverIsSubset(candidate.cover, other.cover)
		}) {
			continue
		}
		filtered = append(filtered, candidate)
	}

	selected := greedyTagFilterCover(filtered, targetClusters)
	if len(selected) == 0 {
		return nil
	}
	result := make([]string, 0, len(selected))
	for _, candidate := range selected {
		result = append(result, candidate.filter)
	}
	sort.Strings(result)
	return result
}

func greedyTagFilterCover(candidates []tagFilterCandidate, targetClusters map[string]struct{}) []tagFilterCandidate {
	var selected []tagFilterCandidate
	covered := make(map[string]struct{})
	for len(covered) < len(targetClusters) {
		bestIndex := -1
		for index, candidate := range candidates {
			if bestIndex == -1 || uncoveredCoverageCount(candidate.cover, covered) > uncoveredCoverageCount(candidates[bestIndex].cover, covered) {
				bestIndex = index
			}
		}
		if bestIndex == -1 || uncoveredCoverageCount(candidates[bestIndex].cover, covered) == 0 {
			return nil
		}
		selected = append(selected, candidates[bestIndex])
		for cluster := range candidates[bestIndex].cover {
			covered[cluster] = struct{}{}
		}
	}
	return selected
}

func candidateCoverIsSubset(candidate, other map[string]struct{}) bool {
	for cluster := range candidate {
		if _, found := other[cluster]; !found {
			return false
		}
	}
	return true
}

func uncoveredCoverageCount(candidate, covered map[string]struct{}) int {
	count := 0
	for cluster := range candidate {
		if _, isCovered := covered[cluster]; !isCovered {
			count++
		}
	}
	return count
}

func (c *metricsClient) queryExpectedMetricPresenceForGPUConfig(metricName string, expectedTags map[string]gpuspec.TagSpec, queryFilter string, fromTS, toTS int64, queryMinMax bool) ([]gpuspec.MetricObservation, error) {
	baseQuery := fmt.Sprintf("%s{%s}", metricName, queryFilter)

	if len(expectedTags) > 0 {
		baseQuery += fmt.Sprintf(" by {%s}", strings.Join(slices.Collect(maps.Keys(expectedTags)), ","))
	}

	queries := []datadogV2.ScalarQuery{buildScalarQuery("avg", "avg:"+baseQuery, datadogV2.METRICSAGGREGATOR_AVG)}
	if queryMinMax {
		// Requesting min/max allows us to check for values outside of the expected ranges. It's not helpful to validate metrics
		// with discrete acceptable values, but we also can't reasonably query all possible values for a metric using the API.
		queries = []datadogV2.ScalarQuery{
			buildScalarQuery("min", "min:"+baseQuery, datadogV2.METRICSAGGREGATOR_MIN),
			buildScalarQuery("max", "max:"+baseQuery, datadogV2.METRICSAGGREGATOR_MAX),
		}
	}

	columns, err := c.runScalarQueries(queries, fromTS, toTS)
	if err != nil {
		return nil, fmt.Errorf("query expected metric presence for %s: %w", metricName, err)
	}

	observations := make([]gpuspec.MetricObservation, 0, len(columns))
	for _, result := range columns {
		for _, value := range result.values {
			if value == nil {
				continue
			}

			observation := gpuspec.MetricObservation{
				Name:  metricName,
				Tags:  []string{},
				Value: value,
			}

			for tag := range expectedTags {
				if isNullishGroupValue(result.tags[tag]) {
					continue
				}
				observation.Tags = append(observation.Tags, tag+":"+result.tags[tag])
			}
			observations = append(observations, observation)
		}
	}

	return observations, nil
}

func (c *metricsClient) listObservedGPUMetricsForGPUConfig(config gpuspec.GPUConfig, queryFilter string, lookbackSeconds int64, metricPrefix string) (map[string]struct{}, error) {
	metrics := map[string]struct{}{}
	options := datadogV2.NewListTagConfigurationsOptionalParameters().
		WithFilterTags(queryFilter).
		WithFilterQueried(true).
		WithWindowSeconds(max(lookbackSeconds, int64(3600))).
		WithPageSize(1000) // we don't have that many metrics, no need to paginate

	response, httpResp, err := c.api.ListTagConfigurations(c.ctx, *options)
	if httpResp != nil && httpResp.Body != nil {
		_ = httpResp.Body.Close()
	}
	if err != nil {
		return nil, fmt.Errorf("list tag configurations for %+v: %w", config, includeAPIErrorBody(err))
	}

	for _, item := range response.Data {
		metricName := ""
		switch {
		case item.Metric != nil:
			metricName = item.Metric.GetId()
		case item.MetricTagConfiguration != nil:
			metricName = item.MetricTagConfiguration.GetId()
		}
		if strings.HasPrefix(metricName, metricPrefix+".") {
			metrics[strings.TrimPrefix(metricName, metricPrefix+".")] = struct{}{}
		}
	}

	return metrics, nil
}

func (c *metricsClient) fetchMetricAllTags(metricName string, wantedTagPrefixes map[string]gpuspec.TagSpec, windowSeconds int64, metricScopeFilter string) ([]string, error) {
	var allTags []string

	for tagPrefix := range wantedTagPrefixes {
		options := datadogV2.NewListTagsByMetricNameOptionalParameters().
			WithFilterMatch(tagPrefix).
			WithFilterIncludeTagValues(true).
			WithPageLimit(1000).
			WithWindowSeconds(windowSeconds).
			WithFilterAllowPartial(true)
		if metricScopeFilter != "" {
			options.WithFilterTags(metricScopeFilter)
		}

		response, httpResp, err := c.api.ListTagsByMetricName(c.ctx, metricName, *options)
		if httpResp != nil && httpResp.Body != nil {
			_ = httpResp.Body.Close()
		}
		if err != nil {
			return nil, fmt.Errorf("fetch tag %s for %s: %w", tagPrefix, metricName, includeAPIErrorBody(err))
		}
		if response.Data == nil || response.Data.Attributes == nil {
			continue
		}

		for _, tag := range response.Data.Attributes.GetTags() {
			// The tag endpoint returns all tags that contain the FilterMatch
			// value, but we're only interested in tags that start with the
			// prefix.
			if strings.HasPrefix(tag, tagPrefix) {
				allTags = append(allTags, tag)
			}
		}
	}

	return allTags, nil
}

func isNullishGroupValue(value string) bool {
	normalizedValue := strings.TrimSpace(strings.ToLower(value))
	return normalizedValue == "" || normalizedValue == "n/a"
}
