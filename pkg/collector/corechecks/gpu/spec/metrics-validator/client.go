package main

import (
	"context"
	"fmt"
	"strings"

	gpuspec "github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/spec"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	datadogV2 "github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
)

var nullishGroupValues = map[string]struct{}{
	"":     {},
	"none": {},
	"null": {},
	"n/a":  {},
}

type scalarColumns struct {
	group  map[string][][]string
	number map[string][]*float64
}

type metricsClient struct {
	api *datadogV2.MetricsApi
	ctx context.Context
}

func newMetricsClient(apiKey, appKey, site string) (*metricsClient, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("api key is required")
	}
	if strings.TrimSpace(appKey) == "" {
		return nil, fmt.Errorf("app key is required")
	}
	if strings.TrimSpace(site) == "" {
		return nil, fmt.Errorf("site is required")
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

	cfg := datadog.NewConfiguration()

	return &metricsClient{
		api: datadogV2.NewMetricsApi(datadog.NewAPIClient(cfg)),
		ctx: ctx,
	}, nil
}

func buildScalarQuery(name, query string) datadogV2.ScalarQuery {
	q := datadogV2.NewMetricsScalarQuery(datadogV2.METRICSAGGREGATOR_AVG, datadogV2.METRICSDATASOURCE_METRICS, query)
	q.SetName(name)
	return datadogV2.MetricsScalarQueryAsScalarQuery(q)
}

func (c *metricsClient) runScalarQueries(queries []datadogV2.ScalarQuery, fromTS, toTS int64) (scalarColumns, error) {
	attrs := datadogV2.NewScalarFormulaRequestAttributes(fromTS*1000, queries, toTS*1000)
	req := datadogV2.NewScalarFormulaRequest(*attrs, datadogV2.SCALARFORMULAREQUESTTYPE_SCALAR_REQUEST)
	body := datadogV2.NewScalarFormulaQueryRequest(*req)

	response, httpResp, err := c.api.QueryScalarData(c.ctx, *body)
	if httpResp != nil && httpResp.Body != nil {
		_ = httpResp.Body.Close()
	}
	if err != nil {
		return scalarColumns{}, fmt.Errorf("query scalar data: %w", err)
	}
	if response.Errors != nil && strings.TrimSpace(*response.Errors) != "" {
		return scalarColumns{}, fmt.Errorf("query scalar data returned errors: %s", strings.TrimSpace(*response.Errors))
	}
	if response.Data == nil || response.Data.Attributes == nil {
		return scalarColumns{
			group:  map[string][][]string{},
			number: map[string][]*float64{},
		}, nil
	}

	return splitScalarColumns(response.Data.Attributes.Columns), nil
}

func splitScalarColumns(columns []datadogV2.ScalarColumn) scalarColumns {
	groupColumns := make(map[string][][]string, len(columns))
	numberColumns := make(map[string][]*float64, len(columns))

	for _, column := range columns {
		switch {
		case column.GroupScalarColumn != nil:
			groupColumns[column.GroupScalarColumn.GetName()] = column.GroupScalarColumn.GetValues()
		case column.DataScalarColumn != nil:
			numberColumns[column.DataScalarColumn.GetName()] = column.DataScalarColumn.GetValues()
		}
	}

	return scalarColumns{
		group:  groupColumns,
		number: numberColumns,
	}
}

func normalizeGroupValue(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func normalizeDeviceMode(slicingMode, virtualizationMode string) string {
	if strings.EqualFold(slicingMode, "mig") {
		return "mig"
	}
	if strings.EqualFold(virtualizationMode, "vgpu") {
		return "vgpu"
	}
	return "physical"
}

func (c *metricsClient) queryDeviceCount(config gpuConfig, fromTS, toTS int64) (int, error) {
	columns, err := c.runScalarQueries(
		[]datadogV2.ScalarQuery{
			buildScalarQuery("q0", fmt.Sprintf("avg:gpu.device.total{%s} by {gpu_uuid}", config.tagFilter())),
		},
		fromTS,
		toTS,
	)
	if err != nil {
		return 0, fmt.Errorf("query device count for %s/%s: %w", config.Architecture, config.DeviceMode, err)
	}

	count := 0
	for _, value := range columns.number["q0"] {
		if value != nil && *value > 0 {
			count++
		}
	}
	return count, nil
}

func (c *metricsClient) discoverLiveGPUConfigs(fromTS, toTS int64) (map[string]struct{}, error) {
	columns, err := c.runScalarQueries(
		[]datadogV2.ScalarQuery{
			buildScalarQuery("q0", "avg:gpu.device.total{*} by {gpu_architecture,gpu_slicing_mode,gpu_virtualization_mode}"),
		},
		fromTS,
		toTS,
	)
	if err != nil {
		return nil, fmt.Errorf("discover live gpu configs: %w", err)
	}

	result := map[string]struct{}{}
	values := columns.number["q0"]
	architectures := columns.group["gpu_architecture"]
	slicingModes := columns.group["gpu_slicing_mode"]
	virtualizationModes := columns.group["gpu_virtualization_mode"]

	for idx, value := range values {
		if value == nil || *value <= 0 {
			continue
		}

		arch := ""
		if idx < len(architectures) {
			arch = strings.ToLower(strings.TrimSpace(normalizeGroupValue(architectures[idx])))
		}
		if arch == "" || arch == "n/a" || arch == "na" || arch == "none" || arch == "unknown" {
			continue
		}

		slicingMode := ""
		if idx < len(slicingModes) {
			slicingMode = normalizeGroupValue(slicingModes[idx])
		}
		virtualizationMode := ""
		if idx < len(virtualizationModes) {
			virtualizationMode = normalizeGroupValue(virtualizationModes[idx])
		}

		config := gpuConfig{
			Architecture: arch,
			DeviceMode:   gpuspecDeviceMode(normalizeDeviceMode(slicingMode, virtualizationMode)),
			IsKnown:      false,
		}
		result[config.key()] = struct{}{}
	}

	return result, nil
}

func (c *metricsClient) queryExpectedMetricsPresenceForGPUConfig(metricNames []string, expectedTagsByMetric map[string]map[string]struct{}, queryFilter string, fromTS, toTS int64) (map[string]struct{}, map[string][]string, error) {
	if len(metricNames) == 0 {
		return map[string]struct{}{}, map[string][]string{}, nil
	}

	queries := make([]datadogV2.ScalarQuery, 0, len(metricNames))
	queryNameToMetric := make(map[string]string, len(metricNames))

	for idx, metricName := range metricNames {
		queryName := fmt.Sprintf("q%d", idx)
		query := fmt.Sprintf("avg:%s{%s}", metricName, queryFilter)

		expectedTags := make([]string, 0, len(expectedTagsByMetric[metricName]))
		for tag := range expectedTagsByMetric[metricName] {
			expectedTags = append(expectedTags, tag)
		}
		if len(expectedTags) > 0 {
			query = fmt.Sprintf("%s by {%s}", query, strings.Join(expectedTags, ","))
		}

		queries = append(queries, buildScalarQuery(queryName, query))
		queryNameToMetric[queryName] = metricName
	}

	columns, err := c.runScalarQueries(queries, fromTS, toTS)
	if err != nil {
		return nil, nil, fmt.Errorf("query expected metrics presence: %w", err)
	}

	presentMetrics := map[string]struct{}{}
	tagFailures := map[string][]string{}

	for queryName, metricName := range queryNameToMetric {
		values := columns.number[queryName]
		presentRowIndexes := make([]int, 0, len(values))
		for rowIdx, value := range values {
			if value != nil {
				presentRowIndexes = append(presentRowIndexes, rowIdx)
			}
		}
		if len(presentRowIndexes) == 0 {
			continue
		}

		presentMetrics[metricName] = struct{}{}
		expectedTags := expectedTagsByMetric[metricName]
		if len(expectedTags) == 0 {
			continue
		}

		nonNullSeen := make(map[string]bool, len(expectedTags))
		for tag := range expectedTags {
			nonNullSeen[tag] = false
		}

		for _, rowIdx := range presentRowIndexes {
			for tag := range expectedTags {
				tagValues := columns.group[tag]
				if rowIdx >= len(tagValues) {
					continue
				}
				normalizedValue := strings.TrimSpace(strings.ToLower(normalizeGroupValue(tagValues[rowIdx])))
				if normalizedValue == "" {
					continue
				}
				if _, nullish := nullishGroupValues[normalizedValue]; nullish {
					continue
				}
				nonNullSeen[tag] = true
			}
		}

		missingTags := make([]string, 0, len(nonNullSeen))
		for tag, seen := range nonNullSeen {
			if !seen {
				missingTags = append(missingTags, tag)
			}
		}
		if len(missingTags) > 0 {
			tagFailures[metricName] = missingTags
		}
	}

	return presentMetrics, tagFailures, nil
}

func (c *metricsClient) listObservedGPUMetricsForGPUConfig(config gpuConfig, lookbackSeconds int64, metricPrefix string) (map[string]struct{}, error) {
	metrics := map[string]struct{}{}
	pageCursor := ""

	for {
		options := datadogV2.NewListTagConfigurationsOptionalParameters().
			WithFilterTags(config.filterExpression()).
			WithFilterQueried(true).
			WithWindowSeconds(max(lookbackSeconds, int64(3600))).
			WithPageSize(1000)
		if pageCursor != "" {
			options.WithPageCursor(pageCursor)
		}

		response, httpResp, err := c.api.ListTagConfigurations(c.ctx, *options)
		if httpResp != nil && httpResp.Body != nil {
			_ = httpResp.Body.Close()
		}
		if err != nil {
			return nil, fmt.Errorf("list tag configurations for %s/%s: %w", config.Architecture, config.DeviceMode, err)
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
				metrics[metricName] = struct{}{}
			}
		}

		pageCursor = ""
		if response.Meta != nil && response.Meta.Pagination != nil && response.Meta.Pagination.NextCursor.IsSet() {
			if nextCursor := response.Meta.Pagination.NextCursor.Get(); nextCursor != nil {
				pageCursor = strings.TrimSpace(*nextCursor)
			}
		}
		if pageCursor == "" {
			break
		}
	}

	return metrics, nil
}

func gpuspecDeviceMode(value string) gpuspec.DeviceMode {
	switch value {
	case "mig":
		return gpuspec.DeviceModeMIG
	case "vgpu":
		return gpuspec.DeviceModeVGPU
	default:
		return gpuspec.DeviceModePhysical
	}
}
