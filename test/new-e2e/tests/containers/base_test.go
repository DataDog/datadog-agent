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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

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

func (suite *baseSuite) testMetric(metricName string, filterTags []string, expectedTags []*regexp.Regexp) {
	suite.Run(fmt.Sprintf("%s{%s}", metricName, strings.Join(filterTags, ",")), func() {
		suite.EventuallyWithTf(func(collect *assert.CollectT) {
			metrics, err := suite.Fakeintake.FilterMetrics(
				metricName,
				fakeintake.WithTags[*aggregator.MetricSeries](filterTags),
			)
			if err != nil {
				collect.Errorf("%w", err)
				return
			}
			if len(metrics) == 0 {
				collect.Errorf("No `%s{%s}` metrics yet", metricName, strings.Join(filterTags, ","))
				return
			}

			// Check tags
			if err := assertTags(metrics[len(metrics)-1].GetTags(), expectedTags); err != nil {
				collect.Errorf("Tags mismatch on `%s`: %w", metricName, err)
				return
			}
		}, 2*time.Minute, 10*time.Second, "Failed finding %s{%s} with proper tags", metricName, strings.Join(filterTags, ","))
	})
}
