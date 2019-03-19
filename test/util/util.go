package util

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"gopkg.in/zorkian/go-datadog-api.v2"
)

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

// RandomString generates a random string of the given size
func RandomString(size int) string {
	b := make([]byte, size)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return string(b)
}

// TimeNowNano returns Unix time with nanosecond precision
func TimeNowNano() float64 {
	return float64(time.Now().UnixNano()) / float64(time.Second)
}

// InitLogging inits default logger
func InitLogging(level string) error {
	err := config.SetupLogger(level, "", "", false, true, false)
	if err != nil {
		return fmt.Errorf("Unable to initiate logger: %s", err)
	}

	return nil
}

// SetHostname sets the hostname
func SetHostname(hostname string) {
	mockConfig := config.Mock()
	mockConfig.Set("hostname", hostname)
}

// BuildQuery builds a datadog query using the given aggregator function and
// metric query (i.e. metric{scope}) to which is applied the default rollup.
func BuildQuery(aggregator, metricQuery string) string {
	rollup := config.Datadog.GetInt("external_metrics_provider.rollup")
	return fmt.Sprintf("%s:%s.rollup(%d)", aggregator, metricQuery, rollup)
}

// BuildQueryWithDefaults is the same as BuildQuery except that it uses the
// default aggregation function.
func BuildQueryWithDefaults(metricQuery string) string {
	agg := config.Datadog.GetString("external_metrics.aggregator")
	return BuildQuery(agg, metricQuery)
}

// BuildSeries builds a time series from the given data points and associate it
// to a datadog query defined by the metric name, aggregation function and
// scope to which is applied the default rollup.
func BuildSeries(name, agg, scope string, points []datadog.DataPoint) datadog.Series {
	query := BuildQuery(agg, fmt.Sprintf("%s{%s}", name, scope))
	return datadog.Series{
		Expression: &query,
		Metric:     &name,
		Points:     points,
		Scope:      &scope,
	}
}

// BuildSeriesWithDefaults is the same as BuildSeries except that it uses the
// default aggregation function.
func BuildSeriesWithDefaults(name, scope string, points []datadog.DataPoint) datadog.Series {
	query := BuildQueryWithDefaults(fmt.Sprintf("%s{%s}", name, scope))
	return datadog.Series{
		Expression: &query,
		Metric:     &name,
		Points:     points,
		Scope:      &scope,
	}
}
