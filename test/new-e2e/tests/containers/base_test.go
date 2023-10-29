// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package containers

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/agent-payload/v5/gogen"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
)

type baseSuite struct {
	suite.Suite

	startTime  time.Time
	endTime    time.Time
	Fakeintake *fakeintake.Client
}

func (suite *baseSuite) SetupSuite() {
	suite.startTime = time.Now()
}

func (suite *baseSuite) TearDownSuite() {
	suite.endTime = time.Now()
}

type testMetricArgs struct {
	filter testMetricFilterArgs
	expect testMetricExpectArgs
}

type testMetricFilterArgs struct {
	name string
	tags []string
}

type testMetricExpectArgs struct {
	tags  *[]string
	value *testMetricExpectValueArgs
}

type testMetricExpectValueArgs struct {
	min float64
	max float64
}

func (suite *baseSuite) testMetric(args *testMetricArgs) {
	suite.Run(fmt.Sprintf("%s{%s}", args.filter.name, strings.Join(args.filter.tags, ",")), func() {
		var expectedTags []*regexp.Regexp
		if args.expect.tags != nil {
			expectedTags = lo.Map(*args.expect.tags, func(tag string, _ int) *regexp.Regexp { return regexp.MustCompile(tag) })
		}

		suite.EventuallyWithTf(func(collect *assert.CollectT) {
			metrics, err := suite.Fakeintake.FilterMetrics(
				args.filter.name,
				fakeintake.WithTags[*aggregator.MetricSeries](args.filter.tags),
			)
			if err != nil {
				collect.Errorf("%w", err)
				return
			}
			if len(metrics) == 0 {
				collect.Errorf("No `%s{%s}` metrics yet", args.filter.name, strings.Join(args.filter.tags, ","))
				return
			}

			// Check tags
			if expectedTags != nil {
				if err := assertTags(metrics[len(metrics)-1].GetTags(), expectedTags); err != nil {
					collect.Errorf("Tags mismatch on `%s`: %w", args.filter.name, err)
				}
			}

			// Check value
			if args.expect.value != nil {
				if lo.CountBy(metrics[len(metrics)-1].GetPoints(), func(v *gogen.MetricPayload_MetricPoint) bool {
					return v.GetValue() >= args.expect.value.min &&
						v.GetValue() <= args.expect.value.max
				}) == 0 {
					collect.Errorf(
						"No value of `%s{%s}` is in the range [%f;%f]",
						args.filter.name,
						strings.Join(args.filter.tags, ","),
						args.expect.value.min,
						args.expect.value.max,
					)
				}
			}
		}, 2*time.Minute, 10*time.Second, "Failed finding %s{%s} with proper tags", args.filter.name, strings.Join(args.filter.tags, ","))
	})
}
