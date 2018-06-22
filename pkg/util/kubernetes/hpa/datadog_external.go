package hpa

import (
	"errors"
	"fmt"
	"time"

	"gopkg.in/zorkian/go-datadog-api.v2"

	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// QueryDatadogExtra converts the metric name and labels from the HPA format into a Datadog metric.
// It returns the last value for a bucket of 5 minutes,
func QueryDatadogExtra(metricName string, tags map[string]string) (int64, error) {
	if metricName == "" || len(tags) == 0 {
		return 0, errors.New("invalid metric to query")
	}
	bucketSize := config.Datadog.GetInt64("hpa_external_metric_bucket_size")
	client := datadog.NewClient(config.Datadog.GetString("api_key"), config.Datadog.GetString("app_key"))
	datadogTags := []string{}

	for key, val := range tags {
		datadogTags = append(datadogTags, fmt.Sprintf("%s:%s", key, val))
	}
	tagEnd := strings.Join(datadogTags, ",")

	// TODO: offer other aggregations than avg.
	query := fmt.Sprintf("avg:%s{%s}", metricName, tagEnd)

	seriesSlice, err := client.QueryMetrics(time.Now().Unix()-bucketSize, time.Now().Unix(), query)

	if err != nil {
		return 0, log.Errorf("Error while executing metric query %s: %s", query, err)
	}
	if len(seriesSlice) < 1 {
		return 0, log.Errorf("Returned series slice empty")
	}
	points := seriesSlice[0].Points

	if len(points) < 1 {
		return 0, log.Errorf("No points in series")
	}
	log.Infof("About to return %#v converted to int64 %#v", points[len(points)-1][1], int64(points[len(points)-1][1]))
	lastValue := int64(points[len(points)-1][1])
	return lastValue, nil
}
