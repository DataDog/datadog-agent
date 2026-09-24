// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

//go:build kubeapiserver

package autoscalers

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
				return nil, errors.New("Rate limit of 300 requests in 3600 seconds")
			},
			[]string{"avg:mymetric{foo:bar}.rollup(30)"},
			nil,
			errors.New("Rate limit of 300 requests in 3600 seconds"),
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
						Expression: pointer.Ptr("mymetric{foo:bar,baz:ar}"),
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
						Expression: pointer.Ptr("mymetric2{foo:baz}"),
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
						Expression: pointer.Ptr("my.aws.metric{ba:bar}"),
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
						Expression: pointer.Ptr("another.metric{foo:empty}"),
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
						Expression: pointer.Ptr("sum:system.io.rkb_s{device:sda} + sum:system.io.rkb_s{device:sdb}by{host}"),
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
						Expression: pointer.Ptr("sum:system.io.rkb_s{device:sda} + sum:system.io.rkb_s{device:sdb}by{host}"),
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
						Expression: pointer.Ptr("mymetric2{foo:baz}"),
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
						Expression: pointer.Ptr("my.aws.metric{ba:bar}"),
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
						Expression: pointer.Ptr("sum:system.io.rkb_s{device:sda} + sum:system.io.rkb_s{device:sdb}by{host}"),
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
						Expression: pointer.Ptr("mymetric2{foo:baz}"),
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
						Expression: pointer.Ptr("my.aws.metric{ba:bar}"),
						QueryIndex: pointer.Ptr(2),
					},
				}, nil
			},
			[]string{"sum:system.io.rkb_s{device:sda} + sum:system.io.rkb_s{device:sdb}by{host}", "mymetric2{foo:baz}", "my.aws.metric{ba:bar}"},
			invalidQueryResults(
				[]string{"sum:system.io.rkb_s{device:sda} + sum:system.io.rkb_s{device:sdb}by{host}", "mymetric2{foo:baz}", "my.aws.metric{ba:bar}"},
				testTime.Unix(),
				"Datadog API response did not match the submitted query batch",
			),
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

func TestValidateDatadogExternalQuery(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		wantErr string
	}{
		{
			name:  "single query",
			query: "avg:requests{env:prod}",
		},
		{
			name:  "commas nested in scope and function",
			query: "moving_rollup(avg:requests{env:prod,service:web}, 300, 'sum')",
		},
		{
			name:  "comma nested in quoted string",
			query: "avg:requests{value:\"one,two\"}",
		},
		{
			name:    "top-level comma",
			query:   "avg:attacker{*},avg:victim{*}",
			wantErr: "query contains a top-level comma",
		},
		{
			name:    "empty query",
			query:   "  ",
			wantErr: "query is empty",
		},
		{
			name:    "unbalanced delimiter",
			query:   "avg:requests{env:prod)",
			wantErr: `unbalanced delimiter ')'`,
		},
		{
			name:    "unclosed delimiter",
			query:   "avg:requests{env:prod",
			wantErr: `query contains an unclosed delimiter '{'`,
		},
		{
			name:    "unterminated quote",
			query:   `avg:requests{value:"unterminated}`,
			wantErr: "query contains an unterminated quoted string",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateDatadogExternalQuery(test.query)
			if test.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.EqualError(t, err, test.wantErr)
		})
	}
}

func TestDatadogExternalQueryFailsClosedOnInvalidQueryIndex(t *testing.T) {
	testTime := time.Now()
	queries := []string{"avg:metric-a{*}", "avg:metric-b{*}"}

	tests := []struct {
		name   string
		series datadog.Series
	}{
		{
			name: "missing index",
			series: datadog.Series{
				Expression: pointer.Ptr(queries[0]),
			},
		},
		{
			name: "negative index",
			series: datadog.Series{
				Expression: pointer.Ptr(queries[0]),
				QueryIndex: pointer.Ptr(-1),
			},
		},
		{
			name: "index outside batch",
			series: datadog.Series{
				Expression: pointer.Ptr(queries[0]),
				QueryIndex: pointer.Ptr(len(queries)),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			datadogClientComp := datadogclientmock.New(t).Comp
			datadogClientComp.SetQueryMetricsFunc(func(int64, int64, string) ([]datadog.Series, error) {
				return []datadog.Series{test.series}, nil
			})

			p := Processor{datadogClient: datadogClientComp}
			points, err := p.queryDatadogExternal(testTime, queries, time.Minute)
			require.NoError(t, err)
			require.Equal(t, invalidQueryResults(queries, testTime.Unix(), "Datadog API response did not match the submitted query batch"), points)
		})
	}
}

func TestMatchDatadogExternalSeriesAcceptsRewrittenExpression(t *testing.T) {
	queries := []string{
		"moving_rollup(avg:system.cpu.idle{cluster:test}, 300, 'avg')",
		"avg:system.cpu.idle{cluster:test} by {host}",
	}
	tests := []struct {
		name   string
		series datadog.Series
		want   string
	}{
		{
			name: "normalized formatting",
			series: datadog.Series{
				Expression: pointer.Ptr("moving_rollup(avg:system.cpu.idle{cluster:test},300,'avg')"),
				QueryIndex: pointer.Ptr(0),
			},
			want: queries[0],
		},
		{
			name: "expanded group",
			series: datadog.Series{
				Expression: pointer.Ptr("avg:system.cpu.idle{cluster:test,host:test-host}"),
				QueryIndex: pointer.Ptr(1),
			},
			want: queries[1],
		},
		{
			name: "missing expression",
			series: datadog.Series{
				QueryIndex: pointer.Ptr(0),
			},
			want: queries[0],
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := matchDatadogExternalSeries(test.series, queries)
			require.NoError(t, err)
			require.Equal(t, test.want, got)
		})
	}
}

func TestQueryExternalMetricRejectsInjectedSubqueriesRegardlessOfOrder(t *testing.T) {
	const attackerQuery = "avg:attacker{*},avg:injected-a{*},avg:injected-b{*}"
	victimQueries := []string{"avg:victim-a{*}", "avg:victim-b{*}"}

	tests := []struct {
		name    string
		queries []string
	}{
		{name: "attacker first", queries: []string{attackerQuery, victimQueries[0], victimQueries[1]}},
		{name: "attacker middle", queries: []string{victimQueries[0], attackerQuery, victimQueries[1]}},
		{name: "attacker last", queries: []string{victimQueries[0], victimQueries[1], attackerQuery}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			datadogClientComp := datadogclientmock.New(t).Comp
			datadogClientComp.SetQueryMetricsFunc(func(_ int64, _ int64, query string) ([]datadog.Series, error) {
				require.Equal(t, strings.Join(victimQueries, ","), query)
				return []datadog.Series{
					{
						Expression: pointer.Ptr(victimQueries[0]),
						QueryIndex: pointer.Ptr(0),
						Metric:     pointer.Ptr("victim-a"),
						Scope:      pointer.Ptr("*"),
						Points:     []datadog.DataPoint{makePoints(100000, 10), makePoints(200000, 10)},
					},
					{
						Expression: pointer.Ptr(victimQueries[1]),
						QueryIndex: pointer.Ptr(1),
						Metric:     pointer.Ptr("victim-b"),
						Scope:      pointer.Ptr("*"),
						Points:     []datadog.DataPoint{makePoints(100000, 20), makePoints(200000, 20)},
					},
				}, nil
			})

			p := Processor{datadogClient: datadogClientComp, parallelQueries: 1}
			points := p.QueryExternalMetric(test.queries, time.Minute)

			require.False(t, points[attackerQuery].Valid)
			require.ErrorContains(t, points[attackerQuery].Error, "top-level comma")
			require.Equal(t, 10.0, points[victimQueries[0]].Value)
			require.True(t, points[victimQueries[0]].Valid)
			require.Equal(t, 20.0, points[victimQueries[1]].Value)
			require.True(t, points[victimQueries[1]].Valid)
		})
	}
}
