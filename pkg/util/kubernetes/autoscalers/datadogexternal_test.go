// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-2020 Datadog, Inc.

// +build kubeapiserver

package autoscalers

import (
	"fmt"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/stretchr/testify/require"
	"gopkg.in/zorkian/go-datadog-api.v2"
)

// TestDatadogExternalQuery tests that the outputs gotten from Datadog are appropriately dealt with.
// Worth noting: We check that the penultimate point is considered and also that even if buckets don't align, we can retrieve the last value.
func TestDatadogExternalQuery(t *testing.T) {
	tests := []struct {
		name       string
		queryfunc  func(from, to int64, query string) ([]datadog.Series, error)
		metricName []string
		points     map[string]Point
		err        error
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
			[]string{"mymetric{foo:bar}"},
			map[string]Point{"mymetric{foo:bar}": {Value: 0, Valid: false}},
			fmt.Errorf("Returned series slice empty"),
		},
		{
			"metricName yields rate limiting error response from Datadog",
			func(int64, int64, string) ([]datadog.Series, error) {
				return nil, fmt.Errorf("Rate limit of 300 requests in 3600 seconds")
			},
			[]string{"avg:mymetric{foo:bar}.rollup(30)"},
			nil,
			fmt.Errorf("Error while executing metric query avg:mymetric{foo:bar}.rollup(30): Rate limit of 300 requests in 3600 seconds"),
		},
		{
			"metrics with different granularities Datadog",
			func(from, to int64, query string) ([]datadog.Series, error) {
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
						Scope:      makePtr("foo:bar,baz:ar"),
						Metric:     makePtr("mymetric"),
						QueryIndex: makePtrInt(0),
					}, {
						Points: []datadog.DataPoint{
							makePartialPoints(10000),
							makePoints(110000, 70),
							makePartialPoints(20000),
							makePoints(300000, 42),
							makePartialPoints(40000),
						},
						Scope:      makePtr("foo:baz"),
						Metric:     makePtr("mymetric2"),
						QueryIndex: makePtrInt(1),
					}, {
						Points: []datadog.DataPoint{
							makePartialPoints(10000),
							makePoints(110000, 3),
							makePartialPoints(20000),
							makePartialPoints(30000),
							makePartialPoints(40000),
						},
						Scope:      makePtr("ba:bar"),
						Metric:     makePtr("my.aws.metric"),
						QueryIndex: makePtrInt(2),
					},
				}, nil
			},
			[]string{"mymetric{foo:bar,baz:ar}", "mymetric2{foo:baz}", "my.aws.metric{ba:bar}"},
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
					Value:     0.0,
					Valid:     false,
					Timestamp: time.Now().Unix(),
				},
			},
			nil,
		},
		{
			"retrieved multiple series for query",
			func(from, to int64, query string) ([]datadog.Series, error) {
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
						Metric:     makePtr("(system.io.rkb_s + system.io.rkb_s)"),
						Scope:      makePtr("device:sda,device:sdb,host:a"),
						QueryIndex: makePtrInt(0),
					},
					{
						Points: []datadog.DataPoint{
							makePoints(100000, 40),
							makePartialPoints(11000),
							makePoints(200000, 23),
							makePoints(300000, 42),
							makePoints(400000, 912),
						},
						Metric:     makePtr("(system.io.rkb_s + system.io.rkb_s)"),
						Scope:      makePtr("device:sda,device:sdb,host:b"),
						QueryIndex: makePtrInt(0),
					},
					{
						Points: []datadog.DataPoint{
							makePartialPoints(10000),
							makePoints(110000, 70),
							makePartialPoints(20000),
							makePoints(300000, 42),
							makePartialPoints(40000),
						},
						Metric:     makePtr("mymetric2"),
						Scope:      makePtr("foo:baz"),
						QueryIndex: makePtrInt(1),
					},
					{
						Points: []datadog.DataPoint{
							makePartialPoints(10000),
							makePoints(110000, 3),
							makePartialPoints(20000),
							makePartialPoints(30000),
							makePartialPoints(40000),
						},
						Metric:     makePtr("my.aws.metric"),
						Scope:      makePtr("ba:bar"),
						QueryIndex: makePtrInt(2),
					},
				}, nil
			},
			[]string{"sum:system.io.rkb_s{device:sda} + sum:system.io.rkb_s{device:sdb}by{host}", "mymetric2{foo:baz}", "my.aws.metric{ba:bar}"},
			map[string]Point{
				"sum:system.io.rkb_s{device:sda} + sum:system.io.rkb_s{device:sdb}by{host}": {
					Value:     42.0,
					Valid:     false,
					Timestamp: time.Now().Unix(),
				},
				"mymetric2{foo:baz}": {
					Value:     70.0,
					Valid:     true,
					Timestamp: 110,
				},
				"my.aws.metric{ba:bar}": {
					Value:     0.0,
					Valid:     false,
					Timestamp: time.Now().Unix(),
				},
			},
			nil,
		},
		{
			"missing queryIndex",
			func(from, to int64, query string) ([]datadog.Series, error) {
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
						Metric:     makePtr("(system.io.rkb_s + system.io.rkb_s)"),
						Scope:      makePtr("device:sda,device:sdb,host:a"),
						QueryIndex: makePtrInt(0),
					},
					{
						Points: []datadog.DataPoint{
							makePartialPoints(10000),
							makePoints(110000, 70),
							makePartialPoints(20000),
							makePoints(300000, 42),
							makePartialPoints(40000),
						},
						Metric: makePtr("mymetric2"),
						Scope:  makePtr("foo:baz"),
					},
					{
						Points: []datadog.DataPoint{
							makePartialPoints(10000),
							makePoints(110000, 3),
							makePartialPoints(20000),
							makePartialPoints(30000),
							makePartialPoints(40000),
						},
						Metric:     makePtr("my.aws.metric"),
						Scope:      makePtr("ba:bar"),
						QueryIndex: makePtrInt(2),
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
					Timestamp: time.Now().Unix(),
				},
				"my.aws.metric{ba:bar}": {
					Value:     0.0,
					Valid:     false,
					Timestamp: time.Now().Unix(),
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
			points, err := p.queryDatadogExternal(test.metricName, config.Datadog.GetInt64("external_metrics_provider.bucket_size"))
			if test.err != nil {
				require.EqualError(t, test.err, err.Error())
			}

			require.Len(t, test.points, len(points))
			for n, p := range test.points {
				require.Equal(t, p.Valid, points[n].Valid)
				require.Equal(t, p.Value, points[n].Value)
				if !p.Valid {
					require.WithinDuration(t, time.Now(), time.Unix(points[n].Timestamp, 0), 5*time.Second)
				}
			}
		})
	}
}
