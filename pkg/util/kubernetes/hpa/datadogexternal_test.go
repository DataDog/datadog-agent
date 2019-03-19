// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build kubeapiserver

package hpa

import (
	"fmt"
	"testing"
	"time"

	testutil "github.com/DataDog/datadog-agent/test/util"
	"github.com/stretchr/testify/require"
	"gopkg.in/zorkian/go-datadog-api.v2"
)

// TestDatadogExternalQuery tests that the outputs gotten from Datadog are appropriately dealt with.
// Worth noting: We check that the penultimate point is considered and also that even if buckets don't align, we can retrieve the last value.
func TestDatadogExternalQuery(t *testing.T) {
	tests := []struct {
		name      string
		queryfunc func(from, to int64, query string) ([]datadog.Series, error)
		queries   []string
		points    map[string]Point
		err       error
	}{
		{
			"metricName is empty",
			nil,
			nil,
			nil,
			nil,
		},
		{
			"metricName yields empty response from Datadog",
			func(from, to int64, query string) ([]datadog.Series, error) {
				return nil, nil
			},
			[]string{testutil.BuildQueryWithDefaults("mymetric{foo:bar}")},
			map[string]Point{testutil.BuildQueryWithDefaults("mymetric{foo:bar}"): {value: 0, valid: false}},
			fmt.Errorf("Returned series slice empty"),
		},
		{
			"metricName yields rate limiting error response from Datadog",
			func(int64, int64, string) ([]datadog.Series, error) {
				return nil, fmt.Errorf("Rate limit of 300 requests in 3600 seconds.")
			},
			[]string{testutil.BuildQueryWithDefaults("mymetric{foo:bar}")},
			nil,
			fmt.Errorf("Error while executing metric query avg:mymetric{foo:bar}.rollup(30): Rate limit of 300 requests in 3600 seconds."),
		},
		{
			"metrics with different granularities Datadog",
			func(from, to int64, query string) ([]datadog.Series, error) {
				return []datadog.Series{
					// Note that points are ordered when we get them from Datadog.
					testutil.BuildSeriesWithDefaults("mymetric", "baz:ar,foo:bar", []datadog.DataPoint{
						makePoints(100000, 40),
						makePartialPoints(11000),
						makePoints(200000, 23),
						makePoints(300000, 42),
						makePoints(400000, 911),
					}),
					testutil.BuildSeriesWithDefaults("mymetric2", "foo:baz", []datadog.DataPoint{
						makePartialPoints(10000),
						makePoints(110000, 70),
						makePartialPoints(20000),
						makePoints(300000, 42),
						makePartialPoints(40000),
					}),
					testutil.BuildSeriesWithDefaults("my.aws.metric", "ba:bar", []datadog.DataPoint{
						makePartialPoints(10000),
						makePoints(110000, 3),
						makePartialPoints(20000),
						makePartialPoints(30000),
						makePartialPoints(40000),
					}),
				}, nil
			},
			[]string{testutil.BuildQueryWithDefaults("mymetric{baz:ar,foo:bar}"), testutil.BuildQueryWithDefaults("mymetric2{foo:baz}"), testutil.BuildQueryWithDefaults("my.aws.metric{ba:bar}")},
			map[string]Point{
				testutil.BuildQueryWithDefaults("mymetric{baz:ar,foo:bar}"): {
					value:     42.0,
					valid:     true,
					timestamp: 300,
				},
				testutil.BuildQueryWithDefaults("mymetric2{foo:baz}"): {
					value:     70.0,
					valid:     true,
					timestamp: 110,
				},
				testutil.BuildQueryWithDefaults("my.aws.metric{ba:bar}"): {
					value:     0.0,
					valid:     false,
					timestamp: time.Now().Unix(),
				},
			},
			nil,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cl := &fakeDatadogClient{
				queryMetricsFunc: test.queryfunc,
			}
			p := Processor{datadogClient: cl}
			points, err := p.queryDatadogExternal(test.queries)
			if test.err != nil {
				require.EqualError(t, test.err, err.Error())
			}

			require.Len(t, test.points, len(points))
			for n, p := range test.points {
				require.Equal(t, p.valid, points[n].valid)
				require.Equal(t, p.value, points[n].value)
				if !p.valid {
					require.WithinDuration(t, time.Now(), time.Unix(points[n].timestamp, 0), 5*time.Second)
				}
			}
		})
	}
}
