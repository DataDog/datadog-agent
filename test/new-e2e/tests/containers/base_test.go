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

type testMetricArgs struct {
	Filter testMetricFilterArgs
	Expect testMetricExpectArgs
}

type testMetricFilterArgs struct {
	Name string
	Tags []string
}

type testMetricExpectArgs struct {
	Tags  *[]string
	Value *testMetricExpectValueArgs
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
	suite.Run(fmt.Sprintf("%s{%s}", args.Filter.Name, strings.Join(args.Filter.Tags, ",")), func() {
		var expectedTags []*regexp.Regexp
		if args.Expect.Tags != nil {
			expectedTags = lo.Map(*args.Expect.Tags, func(tag string, _ int) *regexp.Regexp { return regexp.MustCompile(tag) })
		}

		sendEvent := func(alertType, text string) {
			formattedArgs, err := yaml.Marshal(args)
			suite.Require().NoError(err)

			tags := lo.Map(args.Filter.Tags, func(tag string, _ int) string {
				return "filter_tag_" + tag
			})

			if _, err := suite.datadogClient.PostEvent(&datadog.Event{
				Title: pointer.Ptr(fmt.Sprintf("testMetric %s{%s}", args.Filter.Name, strings.Join(args.Filter.Tags, ","))),
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
				sendEvent("error", fmt.Sprintf("Failed finding %s{%s} with proper tags", args.Filter.Name, strings.Join(args.Filter.Tags, ",")))
			} else {
				sendEvent("success", "All good!")
			}
		}()

		suite.EventuallyWithTf(func(collect *assert.CollectT) {
			myCollect := &myCollectT{
				CollectT: collect,
				errors:   []error{},
			}
			// To enforce the use of myCollect instead
			collect = nil //nolint:ineffassign

			defer func() {
				if len(myCollect.errors) == 0 {
					sendEvent("success", "All good!")
				} else {
					sendEvent("warning", errors.Join(myCollect.errors...).Error())
				}
			}()

			metrics, err := suite.Fakeintake.FilterMetrics(
				args.Filter.Name,
				fakeintake.WithTags[*aggregator.MetricSeries](args.Filter.Tags),
			)
			if err != nil {
				myCollect.Errorf("%w", err)
				return
			}
			if len(metrics) == 0 {
				myCollect.Errorf("No `%s{%s}` metrics yet", args.Filter.Name, strings.Join(args.Filter.Tags, ","))
				return
			}

			// Check tags
			if expectedTags != nil {
				if err := assertTags(metrics[len(metrics)-1].GetTags(), expectedTags); err != nil {
					myCollect.Errorf("Tags mismatch on `%s`: %w", args.Filter.Name, err)
				}
			}

			// Check value
			if args.Expect.Value != nil {
				if lo.CountBy(metrics[len(metrics)-1].GetPoints(), func(v *gogen.MetricPayload_MetricPoint) bool {
					return v.GetValue() >= args.Expect.Value.Min &&
						v.GetValue() <= args.Expect.Value.Max
				}) == 0 {
					myCollect.Errorf(
						"No value of `%s{%s}` is in the range [%f;%f]",
						args.Filter.Name,
						strings.Join(args.Filter.Tags, ","),
						args.Expect.Value.Min,
						args.Expect.Value.Max,
					)
				}
			}
		}, 2*time.Minute, 10*time.Second, "Failed finding %s{%s} with proper tags", args.Filter.Name, strings.Join(args.Filter.Tags, ","))
	})
}
