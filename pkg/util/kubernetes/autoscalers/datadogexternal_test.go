// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

//go:build kubeapiserver

package autoscalers

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gopkg.in/zorkian/go-datadog-api.v2"

	datadogclientmock "github.com/DataDog/datadog-agent/comp/autoscaling/datadogclient/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

// TestDatadogExternalQuery tests that the outputs gotten from Datadog are appropriately dealt with.
// Worth noting: We check that the penultimate point is considered and also that even if buckets don't align, we can retrieve the last value.
func TestDatadogExternalQuery(t *testing.T) {
	testTime := time.Now()

	tests := []struct {
		name           string
		queryfunc      func(from, to int64, query string) ([]datadog.Series, error)
		queries        []string
		expectedPoints map[string]Point
		err            error
	}{
		{
			"metricName is empty",
			nil,
			nil,
			nil,
			nil,
		},
		{
			"metricName yields rate limiting error response from Datadog",
			func(int64, int64, string) ([]datadog.Series, error) {
				return nil, fmt.Errorf("Rate limit of 300 requests in 3600 seconds")
			},
			[]string{"avg:mymetric{foo:bar}.rollup(30)"},
			nil,
			fmt.Errorf("Rate limit of 300 requests in 3600 seconds"),
		},
		{
			"metrics with different granularities Datadog",
			func(int64, int64, string) ([]datadog.Series, error) {
				return []datadog.Series{
					{
						// Note that points are ordered when we get them from Datadog.
						Points: []datadog.DataPoint{
							makePoints(100000, 40),
							makePartialPoints(11000),
							makePoints(200000, 23),
							makePoints(300000, 42),
							makePoints(400000, 911),
						},
						Scope:      pointer.Ptr("foo:bar,baz:ar"),
						Metric:     pointer.Ptr("mymetric"),
						QueryIndex: pointer.Ptr(0),
					},
					{
						Points: []datadog.DataPoint{
							makePartialPoints(10000),
							makePoints(110000, 70),
							makePartialPoints(20000),
							makePoints(300000, 42),
							makePartialPoints(40000),
						},
						Scope:      pointer.Ptr("foo:baz"),
						Metric:     pointer.Ptr("mymetric2"),
						QueryIndex: pointer.Ptr(1),
					},
					{
						Points: []datadog.DataPoint{
							makePartialPoints(10000),
							makePoints(110000, 3),
							makePartialPoints(20000),
							makePartialPoints(30000),
							makePartialPoints(40000),
						},
						Scope:      pointer.Ptr("ba:bar"),
						Metric:     pointer.Ptr("my.aws.metric"),
						QueryIndex: pointer.Ptr(2),
					},
					{
						Points: []datadog.DataPoint{
							makePartialPoints(10000),
							makePartialPoints(20000),
							makePartialPoints(30000),
							makePartialPoints(40000),
						},
						Scope:      pointer.Ptr("foo:empty"),
						Metric:     pointer.Ptr("another.metric"),
						Start:      pointer.Ptr[float64](10000000),
						End:        pointer.Ptr[float64](40000000),
						Interval:   pointer.Ptr(2),
						QueryIndex: pointer.Ptr(3),
					},
				}, nil
			},
			[]string{"mymetric{foo:bar,baz:ar}", "mymetric2{foo:baz}", "my.aws.metric{ba:bar}", "another.metric{foo:empty}"},
			map[string]Point{
				"mymetric{foo:bar,baz:ar}": {
					Value:     42.0,
					Valid:     true,
					Timestamp: 300,
				},
				"mymetric2{foo:baz}": {
					Value:     70.0,
					Valid:     true,
					Timestamp: 110,
				},
				"my.aws.metric{ba:bar}": {
					Value:     3.0,
					Valid:     true,
					Timestamp: 110,
				},
				"another.metric{foo:empty}": {
					Valid:     false,
					Timestamp: testTime.Unix(),
					Error:     NewProcessingError("only null values found in API response (4 points), check data is available in the last 300 seconds (interval was 2)"),
				},
			},
			nil,
		},
		{
			"retrieved multiple series for query",
			func(int64, int64, string) ([]datadog.Series, error) {
				return []datadog.Series{
					{
						// Note that points are ordered when we get them from Datadog.
						Points: []datadog.DataPoint{
							makePoints(100000, 40),
							makePartialPoints(11000),
							makePoints(200000, 23),
							makePoints(300000, 42),
							makePoints(400000, 911),
						},
						Metric:     pointer.Ptr("(system.io.rkb_s + system.io.rkb_s)"),
						Scope:      pointer.Ptr("device:sda,device:sdb,host:a"),
						QueryIndex: pointer.Ptr(0),
					},
					{
						Points: []datadog.DataPoint{
							makePoints(100000, 40),
							makePartialPoints(11000),
							makePoints(200000, 23),
							makePoints(300000, 42),
							makePoints(400000, 912),
						},
						Metric:     pointer.Ptr("(system.io.rkb_s + system.io.rkb_s)"),
						Scope:      pointer.Ptr("device:sda,device:sdb,host:b"),
						QueryIndex: pointer.Ptr(0),
					},
					{
						Points: []datadog.DataPoint{
							makePartialPoints(10000),
							makePoints(110000, 70),
							makePartialPoints(20000),
							makePoints(300000, 42),
							makePartialPoints(40000),
						},
						Metric:     pointer.Ptr("mymetric2"),
						Scope:      pointer.Ptr("foo:baz"),
						QueryIndex: pointer.Ptr(1),
					},
					{
						Points: []datadog.DataPoint{
							makePartialPoints(10000),
							makePoints(110000, 3),
							makePartialPoints(20000),
							makePartialPoints(30000),
							makePartialPoints(40000),
						},
						Metric:     pointer.Ptr("my.aws.metric"),
						Scope:      pointer.Ptr("ba:bar"),
						QueryIndex: pointer.Ptr(2),
					},
				}, nil
			},
			[]string{"sum:system.io.rkb_s{device:sda} + sum:system.io.rkb_s{device:sdb}by{host}", "mymetric2{foo:baz}", "my.aws.metric{ba:bar}"},
			map[string]Point{
				"sum:system.io.rkb_s{device:sda} + sum:system.io.rkb_s{device:sdb}by{host}": {
					Value:     0,
					Valid:     false,
					Timestamp: testTime.Unix(),
					Error:     NewProcessingError("multiple series found. Please change your query to return a single serie"),
				},
				"mymetric2{foo:baz}": {
					Value:     70.0,
					Valid:     true,
					Timestamp: 110,
				},
				"my.aws.metric{ba:bar}": {
					Value:     3.0,
					Valid:     true,
					Timestamp: 110,
				},
			},
			nil,
		},
		{
			"missing queryIndex",
			func(int64, int64, string) ([]datadog.Series, error) {
				return []datadog.Series{
					{
						// Note that points are ordered when we get them from Datadog.
						Points: []datadog.DataPoint{
							makePoints(100000, 40),
							makePartialPoints(11000),
							makePoints(200000, 23),
							makePoints(300000, 42),
							makePoints(400000, 911),
						},
						Metric:     pointer.Ptr("(system.io.rkb_s + system.io.rkb_s)"),
						Scope:      pointer.Ptr("device:sda,device:sdb,host:a"),
						QueryIndex: pointer.Ptr(0),
					},
					{
						Points: []datadog.DataPoint{
							makePartialPoints(10000),
							makePoints(110000, 70),
							makePartialPoints(20000),
							makePoints(300000, 42),
							makePartialPoints(40000),
						},
						Metric: pointer.Ptr("mymetric2"),
						Scope:  pointer.Ptr("foo:baz"),
					},
					{
						Points: []datadog.DataPoint{
							makePartialPoints(10000),
							makePoints(110000, 3),
							makePartialPoints(20000),
							makePartialPoints(30000),
							makePartialPoints(40000),
						},
						Metric:     pointer.Ptr("my.aws.metric"),
						Scope:      pointer.Ptr("ba:bar"),
						QueryIndex: pointer.Ptr(2),
					},
				}, nil
			},
			[]string{"sum:system.io.rkb_s{device:sda} + sum:system.io.rkb_s{device:sdb}by{host}", "mymetric2{foo:baz}", "my.aws.metric{ba:bar}"},
			map[string]Point{
				"sum:system.io.rkb_s{device:sda} + sum:system.io.rkb_s{device:sdb}by{host}": {
					Value:     42.0,
					Valid:     true,
					Timestamp: 300,
				},
				"mymetric2{foo:baz}": {
					Value:     0.0,
					Valid:     false,
					Timestamp: testTime.Unix(),
					Error:     NewProcessingError("no serie was found for this query in API Response, check Cluster Agent logs for QueryIndex errors"),
				},
				"my.aws.metric{ba:bar}": {
					Value:     3.0,
					Valid:     true,
					Timestamp: 110,
				},
			},
			nil,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mockConfig := configmock.New(t)
			datadogClientComp := datadogclientmock.New(t).Comp
			datadogClientComp.SetQueryMetricsFunc(test.queryfunc)
			p := Processor{datadogClient: datadogClientComp}
			points, err := p.queryDatadogExternal(testTime, test.queries, time.Duration(mockConfig.GetInt64("external_metrics_provider.bucket_size"))*time.Second)
			if test.err != nil {
				assert.EqualError(t, test.err, err.Error())
			}

			assert.EqualValues(t, test.expectedPoints, points)
		})
	}
}
