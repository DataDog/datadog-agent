package main

import (
	"context"
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

	return &metricsClient{
		api: datadogV2.NewMetricsApi(datadog.NewAPIClient(datadog.NewConfiguration())),
		ctx: ctx,
	}, nil
}

func buildScalarQuery(name, query string) datadogV2.ScalarQuery {
	q := datadogV2.NewMetricsScalarQuery(datadogV2.METRICSAGGREGATOR_AVG, datadogV2.METRICSDATASOURCE_METRICS, query)
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
		return nil, fmt.Errorf("query scalar data returned no data")
	}

	return splitScalarColumns(response.Data.Attributes.Columns)
}

func splitScalarColumns(columns []datadogV2.ScalarColumn) ([]scalarResult, error) {
	var results []scalarResult
	numResults := 0

	for _, column := range columns {
		if column.DataScalarColumn != nil {
			if results == nil {
				numResults = len(column.DataScalarColumn.GetValues())
				results = make([]scalarResult, numResults)
				for idx := range results {
					results[idx] = scalarResult{
						tags:   map[string]string{},
						values: map[string]*float64{},
					}
				}
			}

			for idx, value := range column.DataScalarColumn.GetValues() {
				if idx >= numResults {
					return nil, fmt.Errorf("query scalar data returned unexpected number of results, expected %d but got %d", numResults, idx+1)
				}

				if value != nil {
					valueCopy := *value
					results[idx].values[column.DataScalarColumn.GetName()] = &valueCopy
				}
			}
		} else if column.GroupScalarColumn != nil {
			if results == nil {
				numResults = len(column.GroupScalarColumn.GetValues())
				results = make([]scalarResult, numResults)
				for idx := range results {
					results[idx] = scalarResult{
						tags:   map[string]string{},
						values: map[string]*float64{},
					}
				}
			}

			for idx, tags := range column.GroupScalarColumn.GetValues() {
				if idx >= numResults {
					return nil, fmt.Errorf("query scalar data returned unexpected number of results, expected %d but got %d", numResults, idx+1)
				}

				if tags != nil {
					results[idx].tags[column.GroupScalarColumn.GetName()] = strings.Join(tags, ",")
				}
			}
		} else {
			return nil, fmt.Errorf("query scalar data returned unexpected column type: %T", column)
		}
	}

	return results, nil
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

	return len(columns), nil
}

func (c *metricsClient) discoverLiveGPUConfigs(fromTS, toTS int64) ([]gpuConfig, error) {
	deviceTotalQuery := "q0"
	columns, err := c.runScalarQueries(
		[]datadogV2.ScalarQuery{
			buildScalarQuery(deviceTotalQuery, "avg:gpu.device.total{*} by {gpu_architecture,gpu_slicing_mode,gpu_virtualization_mode}"),
		},
		fromTS,
		toTS,
	)
	if err != nil {
		return nil, fmt.Errorf("discover live gpu configs: %w", err)
	}

	var configs []gpuConfig
	for _, result := range columns {
		value, found := result.values[deviceTotalQuery]
		if !found || value == nil || *value <= 0 {
			continue
		}

		arch := result.tags["gpu_architecture"]
		slicingMode := result.tags["gpu_slicing_mode"]
		virtualizationMode := result.tags["gpu_virtualization_mode"]

		if isNullishGroupValue(arch) {
			continue
		}

		configs = append(configs, gpuConfig{
			Architecture: arch,
			DeviceMode:   gpuspecDeviceMode(normalizeDeviceMode(slicingMode, virtualizationMode)),
			IsKnown:      false,
		})
	}

	return configs, nil
}

func (c *metricsClient) queryExpectedMetricsPresenceForGPUConfig(metricNames []string, expectedTagsByMetric map[string]map[string]struct{}, queryFilter string, fromTS, toTS int64) (map[string]gpuspec.MetricObservation, error) {
	if len(metricNames) == 0 {
		return map[string]gpuspec.MetricObservation{}, nil
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
		return nil, fmt.Errorf("query expected metrics presence: %w", err)
	}

	observations := make(map[string]gpuspec.MetricObservation, len(metricNames))
	for _, metricName := range metricNames {
		observations[metricName] = gpuspec.MetricObservation{
			Name: metricName,
			Tags: []string{},
		}
	}

	for _, result := range columns {
		for queryName, value := range result.values {
			if value == nil {
				continue
			}

			metricName, found := queryNameToMetric[queryName]
			if !found {
				continue
			}

			observation := observations[metricName]
			for tag := range expectedTagsByMetric[metricName] {
				if isNullishGroupValue(result.tags[tag]) {
					continue
				}
				observation.Tags = append(observation.Tags, tag+":"+result.tags[tag])
			}

			observations[metricName] = observation
		}
	}

	return observations, nil
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

func isNullishGroupValue(value string) bool {
	normalizedValue := strings.TrimSpace(strings.ToLower(value))
	return normalizedValue == "" || normalizedValue == "none" || normalizedValue == "null" || normalizedValue == "n/a"
}
