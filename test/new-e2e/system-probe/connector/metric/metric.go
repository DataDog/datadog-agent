// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package metric is responsible for emitting metrics to Datadog
package metric

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
)

// SubmitExecutionMetric accepts metrics and submits it to Datadog.
func SubmitExecutionMetric(metricBody datadogV2.MetricPayload) error {
	if _, ok := os.LookupEnv("DD_API_KEY"); !ok {
		fmt.Fprintf(os.Stderr, "skipping sending metric because DD_API_KEY not present")
		return nil
	}

	ctx := datadog.NewDefaultContext(context.Background())
	configuration := datadog.NewConfiguration()
	apiClient := datadog.NewAPIClient(configuration)
	api := datadogV2.NewMetricsApi(apiClient)
	resp, r, err := api.SubmitMetrics(ctx, metricBody, *datadogV2.NewSubmitMetricsOptionalParameters())

	if err != nil {
		fmt.Fprintf(os.Stderr, "Full HTTP response: %v\n", r)
		return fmt.Errorf("error when calling `MetricsApi.SubmitMetrics`: %v", err)
	}

	responseContent, _ := json.MarshalIndent(resp, "", "  ")
	fmt.Fprintf(os.Stdout, "Response from `MetricsApi.SubmitMetrics`:\n%s\n", responseContent)

	return nil
}
