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

func (c *metricsClient) queryExpectedMetricPresenceForGPUConfig(metricName string, expectedTags map[string]struct{}, queryFilter string, fromTS, toTS int64) ([]gpuspec.MetricObservation, error) {
	query := fmt.Sprintf("avg:%s{%s}", metricName, queryFilter)
	expectedTagNames := make([]string, 0, len(expectedTags))
	for tag := range expectedTags {
		expectedTagNames = append(expectedTagNames, tag)
	}
	if len(expectedTagNames) > 0 {
		query = fmt.Sprintf("%s by {%s}", query, strings.Join(expectedTagNames, ","))
	}

	columns, err := c.runScalarQueries([]datadogV2.ScalarQuery{buildScalarQuery("q0", query, datadogV2.METRICSAGGREGATOR_AVG)}, fromTS, toTS)
	if err != nil {
		return nil, fmt.Errorf("query expected metric presence for %s: %w", metricName, err)
	}

	observations := make([]gpuspec.MetricObservation, 0, len(columns))
	for _, result := range columns {
		value, found := result.values["q0"]
		if !found || value == nil {
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

	return observations, nil
}

func (c *metricsClient) queryMetricValuesForGPUConfig(metricName string, queryFilter string, fromTS, toTS int64) ([]float64, error) {
	queries := []datadogV2.ScalarQuery{
		buildScalarQuery("min", fmt.Sprintf("min:%s{%s}", metricName, queryFilter), datadogV2.METRICSAGGREGATOR_MIN),
		buildScalarQuery("max", fmt.Sprintf("max:%s{%s}", metricName, queryFilter), datadogV2.METRICSAGGREGATOR_MAX),
	}

	columns, err := c.runScalarQueries(queries, fromTS, toTS)
	if err != nil {
		return nil, fmt.Errorf("query metric values for %s: %w", metricName, err)
	}

	values := make([]float64, 0, len(columns)*2)
	for _, result := range columns {
		for _, queryName := range []string{"min", "max"} {
			value, found := result.values[queryName]
			if !found || value == nil {
				continue
			}
			values = append(values, *value)
		}
	}

	return values, nil
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

func isNullishGroupValue(value string) bool {
	normalizedValue := strings.TrimSpace(strings.ToLower(value))
	return normalizedValue == "" || normalizedValue == "n/a"
}
