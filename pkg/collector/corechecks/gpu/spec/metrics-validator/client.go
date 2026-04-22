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
	"slices"
	"strings"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	datadogV2 "github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"

	gpuspec "github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/spec"
)

type metricsClient struct {
	api *datadogV2.MetricsApi
	ctx context.Context
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

func (c *metricsClient) runScalarQueries(queries []datadogV2.ScalarQuery, fromTS, toTS int64) ([]scalarResult, error) {
	attrs := datadogV2.NewScalarFormulaRequestAttributes(fromTS*1000, queries, toTS*1000)
	req := datadogV2.NewScalarFormulaRequest(*attrs, datadogV2.SCALARFORMULAREQUESTTYPE_SCALAR_REQUEST)
	body := datadogV2.NewScalarFormulaQueryRequest(*req)

	response, httpResp, err := c.api.QueryScalarData(c.ctx, *body)
	if httpResp != nil && httpResp.Body != nil {
		_ = httpResp.Body.Close()
	}
	if err != nil {
		return nil, fmt.Errorf("query scalar data: %w", err)
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

func (c *metricsClient) queryDeviceCount(config gpuspec.GPUConfig, fromTS, toTS int64) (int, error) {
	columns, err := c.runScalarQueries(
		[]datadogV2.ScalarQuery{
			buildScalarQuery("q0", fmt.Sprintf("avg:gpu.device.total{%s} by {gpu_uuid}", config.TagFilter()), datadogV2.METRICSAGGREGATOR_AVG),
		},
		fromTS,
		toTS,
	)
	if err != nil {
		return 0, fmt.Errorf("query device count for %s/%s: %w", config.Architecture, config.DeviceMode, err)
	}

	return len(columns), nil
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

func (c *metricsClient) listObservedGPUMetricsForGPUConfig(config gpuspec.GPUConfig, lookbackSeconds int64, metricPrefix string) (map[string]struct{}, error) {
	metrics := map[string]struct{}{}
	options := datadogV2.NewListTagConfigurationsOptionalParameters().
		WithFilterTags(config.TagFilter()).
		WithFilterQueried(true).
		WithWindowSeconds(max(lookbackSeconds, int64(3600))).
		WithPageSize(1000) // we don't have that many metrics, no need to paginate

	response, httpResp, err := c.api.ListTagConfigurations(c.ctx, *options)
	if httpResp != nil && httpResp.Body != nil {
		_ = httpResp.Body.Close()
	}
	if err != nil {
		return nil, fmt.Errorf("list tag configurations for %+v: %w", config, err)
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
			return nil, fmt.Errorf("fetch tag %s for %s: %w", tagPrefix, metricName, err)
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
