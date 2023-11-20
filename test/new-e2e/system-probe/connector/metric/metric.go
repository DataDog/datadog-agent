package metric

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/DataDog/datadog-api-client-go/api/v1/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
)

func SubmitExecutionMetric(metric datadogV2.MetricPayload, failType, result string) error {
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
