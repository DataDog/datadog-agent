// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package containers

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gopkg.in/yaml.v3"
	"gopkg.in/zorkian/go-datadog-api.v2"

	"github.com/DataDog/agent-payload/v5/gogen"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner/parameters"
)

type baseSuite struct {
	suite.Suite

	startTime     time.Time
	endTime       time.Time
	datadogClient *datadog.Client
	Fakeintake    *fakeintake.Client
	clusterName   string
}

func (suite *baseSuite) SetupSuite() {
	apiKey, err := runner.GetProfile().SecretStore().Get(parameters.APIKey)
	suite.Require().NoError(err)
	appKey, err := runner.GetProfile().SecretStore().Get(parameters.APPKey)
	suite.Require().NoError(err)
	suite.datadogClient = datadog.NewClient(apiKey, appKey)

	suite.startTime = time.Now()
}

func (suite *baseSuite) TearDownSuite() {
	suite.endTime = time.Now()
}

func (suite *baseSuite) BeforeTest(suiteName, testName string) {
	suite.T().Logf("START  %s/%s %s", suiteName, testName, time.Now())
}

func (suite *baseSuite) AfterTest(suiteName, testName string) {
	suite.T().Logf("FINISH %s/%s %s", suiteName, testName, time.Now())
}

type testMetricArgs struct {
	Filter   testMetricFilterArgs
	Expect   testMetricExpectArgs
	Optional testMetricExpectArgs
}

type testMetricFilterArgs struct {
	Name string
	// Tags are used to filter the metrics
	// Regexes are supported
	Tags []string
}

type testMetricExpectArgs struct {
	// Tags are the tags expected to be present
	// Regexes are supported
	Tags                 *[]string
	Value                *testMetricExpectValueArgs
	AcceptUnexpectedTags bool
}

type testMetricExpectValueArgs struct {
	Min float64
	Max float64
}

// myCollectT does nothing more than "github.com/stretchr/testify/assert".CollectT
// It’s used only to get access to `errors` field which is otherwise private.
type myCollectT struct {
	*assert.CollectT

	errors []error
}

func (mc *myCollectT) Errorf(format string, args ...interface{}) {
	mc.errors = append(mc.errors, fmt.Errorf(format, args...))
	mc.CollectT.Errorf(format, args...)
}

func (suite *baseSuite) testMetric(args *testMetricArgs) {
	prettyMetricQuery := fmt.Sprintf("%s{%s}", args.Filter.Name, strings.Join(args.Filter.Tags, ","))

	suite.Run("metric   "+prettyMetricQuery, func() {
		var expectedTags []*regexp.Regexp
		if args.Expect.Tags != nil {
			expectedTags = lo.Map(*args.Expect.Tags, func(tag string, _ int) *regexp.Regexp { return regexp.MustCompile(tag) })
		}

		var optionalTags []*regexp.Regexp
		if args.Optional.Tags != nil {
			optionalTags = lo.Map(*args.Optional.Tags, func(tag string, _ int) *regexp.Regexp { return regexp.MustCompile(tag) })
		}

		sendEvent := func(alertType, text string) {
			formattedArgs, err := yaml.Marshal(args)
			suite.Require().NoError(err)

			tags := lo.Map(args.Filter.Tags, func(tag string, _ int) string {
				return "filter_tag_" + tag
			})

			if _, err := suite.datadogClient.PostEvent(&datadog.Event{
				Title: pointer.Ptr(fmt.Sprintf("testMetric %s", prettyMetricQuery)),
				Text: pointer.Ptr(fmt.Sprintf(`%%%%%%
### Result

`+"```"+`
%s
`+"```"+`

### Query

`+"```"+`
%s
`+"```"+`
 %%%%%%`, text, formattedArgs)),
				AlertType: &alertType,
				Tags: append([]string{
					"app:agent-new-e2e-tests-containers",
					"cluster_name:" + suite.clusterName,
					"metric:" + args.Filter.Name,
					"test:" + suite.T().Name(),
				}, tags...),
			}); err != nil {
				suite.T().Logf("Failed to post event: %s", err)
			}
		}

		defer func() {
			if suite.T().Failed() {
				sendEvent("error", fmt.Sprintf("Failed finding %s with proper tags and value", prettyMetricQuery))
			} else {
				sendEvent("success", "All good!")
			}
		}()

		suite.EventuallyWithTf(func(collect *assert.CollectT) {
			c := &myCollectT{
				CollectT: collect,
				errors:   []error{},
			}
			// To enforce the use of myCollectT instead
			collect = nil //nolint:ineffassign

			defer func() {
				if len(c.errors) == 0 {
					sendEvent("success", "All good!")
				} else {
					sendEvent("warning", errors.Join(c.errors...).Error())
				}
			}()

			regexTags := lo.Map(args.Filter.Tags, func(tag string, _ int) *regexp.Regexp {
				return regexp.MustCompile(tag)
			})

			metrics, err := suite.Fakeintake.FilterMetrics(
				args.Filter.Name,
				fakeintake.WithMatchingTags[*aggregator.MetricSeries](regexTags),
			)
			// Can be replaced by require.NoErrorf(…) once https://github.com/stretchr/testify/pull/1481 is merged
			if !assert.NoErrorf(c, err, "Failed to query fake intake") {
				return
			}
			// Can be replaced by require.NoEmptyf(…) once https://github.com/stretchr/testify/pull/1481 is merged
			if !assert.NotEmptyf(c, metrics, "No `%s` metrics yet", prettyMetricQuery) {
				return
			}

			// Check tags
			if expectedTags != nil {
				err := assertTags(metrics[len(metrics)-1].GetTags(), expectedTags, optionalTags, args.Expect.AcceptUnexpectedTags)
				assert.NoErrorf(c, err, "Tags mismatch on `%s`", prettyMetricQuery)
			}

			// Check value
			if args.Expect.Value != nil {
				assert.NotEmptyf(c, lo.Filter(metrics[len(metrics)-1].GetPoints(), func(v *gogen.MetricPayload_MetricPoint, _ int) bool {
					return v.GetValue() >= args.Expect.Value.Min &&
						v.GetValue() <= args.Expect.Value.Max
				}), "No value of `%s` is in the range [%f;%f]: %v",
					prettyMetricQuery,
					args.Expect.Value.Min,
					args.Expect.Value.Max,
					lo.Map(metrics[len(metrics)-1].GetPoints(), func(v *gogen.MetricPayload_MetricPoint, _ int) float64 {
						return v.GetValue()
					}),
				)
			}
		}, 2*time.Minute, 10*time.Second, "Failed finding `%s` with proper tags and value", prettyMetricQuery)
	})
}

type testLogArgs struct {
	Filter testLogFilterArgs
	Expect testLogExpectArgs
}

type testLogFilterArgs struct {
	Service string
	Tags    []string
}

type testLogExpectArgs struct {
	Tags    *[]string
	Message string
}

func (suite *baseSuite) testLog(args *testLogArgs) {
	prettyLogQuery := fmt.Sprintf("%s{%s}", args.Filter.Service, strings.Join(args.Filter.Tags, ","))

	suite.Run("log   "+prettyLogQuery, func() {
		var expectedTags []*regexp.Regexp
		if args.Expect.Tags != nil {
			expectedTags = lo.Map(*args.Expect.Tags, func(tag string, _ int) *regexp.Regexp { return regexp.MustCompile(tag) })
		}

		var expectedMessage *regexp.Regexp
		if args.Expect.Message != "" {
			expectedMessage = regexp.MustCompile(args.Expect.Message)
		}

		sendEvent := func(alertType, text string) {
			formattedArgs, err := yaml.Marshal(args)
			suite.Require().NoError(err)

			tags := lo.Map(args.Filter.Tags, func(tag string, _ int) string {
				return "filter_tag_" + tag
			})

			if _, err := suite.datadogClient.PostEvent(&datadog.Event{
				Title: pointer.Ptr(fmt.Sprintf("testLog %s", prettyLogQuery)),
				Text: pointer.Ptr(fmt.Sprintf(`%%%%%%
### Result

`+"```"+`
%s
`+"```"+`

### Query

`+"```"+`
%s
`+"```"+`
 %%%%%%`, text, formattedArgs)),
				AlertType: &alertType,
				Tags: append([]string{
					"app:agent-new-e2e-tests-containers",
					"cluster_name:" + suite.clusterName,
					"log_service:" + args.Filter.Service,
					"test:" + suite.T().Name(),
				}, tags...),
			}); err != nil {
				suite.T().Logf("Failed to post event: %s", err)
			}
		}

		defer func() {
			if suite.T().Failed() {
				sendEvent("error", fmt.Sprintf("Failed finding %s with proper tags and message", prettyLogQuery))
			} else {
				sendEvent("success", "All good!")
			}
		}()

		suite.EventuallyWithTf(func(collect *assert.CollectT) {
			c := &myCollectT{
				CollectT: collect,
				errors:   []error{},
			}
			// To enforce the use of myCollectT instead
			collect = nil //nolint:ineffassign

			defer func() {
				if len(c.errors) == 0 {
					sendEvent("success", "All good!")
				} else {
					sendEvent("warning", errors.Join(c.errors...).Error())
				}
			}()

			regexTags := lo.Map(args.Filter.Tags, func(tag string, _ int) *regexp.Regexp {
				return regexp.MustCompile(tag)
			})

			logs, err := suite.Fakeintake.FilterLogs(
				args.Filter.Service,
				fakeintake.WithMatchingTags[*aggregator.Log](regexTags),
			)
			// Can be replaced by require.NoErrorf(…) once https://github.com/stretchr/testify/pull/1481 is merged
			if !assert.NoErrorf(c, err, "Failed to query fake intake") {
				return
			}
			// Can be replaced by require.NoEmptyf(…) once https://github.com/stretchr/testify/pull/1481 is merged
			if !assert.NotEmptyf(c, logs, "No `%s` logs yet", prettyLogQuery) {
				return
			}

			// Check tags
			if expectedTags != nil {
				err := assertTags(logs[len(logs)-1].GetTags(), expectedTags, []*regexp.Regexp{}, false)
				assert.NoErrorf(c, err, "Tags mismatch on `%s`", prettyLogQuery)
			}

			// Check message
			if args.Expect.Message != "" {
				assert.NotEmptyf(c, lo.Filter(logs, func(m *aggregator.Log, _ int) bool {
					return expectedMessage.MatchString(m.Message)
				}), "No log of `%s` is matching %q",
					prettyLogQuery,
					args.Expect.Message,
				)
			}
		}, 2*time.Minute, 10*time.Second, "Failed finding `%s` with proper tags and message", prettyLogQuery)
	})
}
