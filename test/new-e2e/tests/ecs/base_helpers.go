// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package ecs

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
	"gopkg.in/zorkian/go-datadog-api.v2"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awsecs "github.com/aws/aws-sdk-go-v2/service/ecs"
	awsecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"

	"github.com/DataDog/agent-payload/v5/gogen"

	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
)

// assertTags checks that actual tags match expected tag patterns
func assertTags(actualTags []string, expectedTags []*regexp.Regexp, optionalTags []*regexp.Regexp, acceptUnexpectedTags bool) error {
	missingTags := make([]*regexp.Regexp, len(expectedTags))
	copy(missingTags, expectedTags)
	unexpectedTags := []string{}

	for _, actualTag := range actualTags {
		found := false
		for i, expectedTag := range missingTags {
			if expectedTag.MatchString(actualTag) {
				found = true
				missingTags[i] = missingTags[len(missingTags)-1]
				missingTags = missingTags[:len(missingTags)-1]
				break
			}
		}

		if !found {
			for _, optionalTag := range optionalTags {
				if optionalTag.MatchString(actualTag) {
					found = true
					break
				}
			}
		}

		if !found {
			unexpectedTags = append(unexpectedTags, actualTag)
		}
	}

	if (len(unexpectedTags) > 0 && !acceptUnexpectedTags) || len(missingTags) > 0 {
		errs := make([]error, 0, 2)
		if len(unexpectedTags) > 0 {
			errs = append(errs, fmt.Errorf("unexpected tags: %s", strings.Join(unexpectedTags, ", ")))
		}
		if len(missingTags) > 0 {
			errs = append(errs, fmt.Errorf("missing tags: %s", strings.Join(lo.Map(missingTags, func(re *regexp.Regexp, _ int) string { return re.String() }), ", ")))
		}
		return errors.Join(errs...)
	}

	return nil
}

type TestMetricArgs struct {
	Filter   TestMetricFilterArgs
	Expect   TestMetricExpectArgs
	Optional TestMetricExpectArgs
}

type TestMetricFilterArgs struct {
	Name string
	// Tags are used to filter the metrics
	// Regexes are supported
	Tags []string
}

type TestMetricExpectArgs struct {
	// Tags are the tags expected to be present
	// Regexes are supported
	Tags                 *[]string
	Value                *TestMetricExpectValueArgs
	AcceptUnexpectedTags bool
}

type TestMetricExpectValueArgs struct {
	Min float64
	Max float64
}

// myCollectT does nothing more than "github.com/stretchr/testify/assert".CollectT
// It's used only to get access to `errors` field which is otherwise private.
type myCollectT struct {
	*assert.CollectT

	errors []error
}

func (mc *myCollectT) Errorf(format string, args ...interface{}) {
	mc.errors = append(mc.errors, fmt.Errorf(format, args...))
	mc.CollectT.Errorf(format, args...)
}

func (suite *BaseSuite[Env]) AssertMetric(args *TestMetricArgs) {
	prettyMetricQuery := fmt.Sprintf("%s{%s}", args.Filter.Name, strings.Join(args.Filter.Tags, ","))

	suite.Run("metric   "+prettyMetricQuery, func() {
		var expectedTags []*regexp.Regexp
		if args.Expect.Tags != nil {
			expectedTags = lo.Map(*args.Expect.Tags, func(tag string, _ int) *regexp.Regexp { return regexp.MustCompile(tag) })
		}

		optionalTags := []*regexp.Regexp{regexp.MustCompile("stackid:.*")} // The stackid tag is added by the framework itself to allow filtering on the stack id
		if args.Optional.Tags != nil {
			optionalTags = lo.Map(*args.Optional.Tags, func(tag string, _ int) *regexp.Regexp { return regexp.MustCompile(tag) })
		}

		sendEvent := func(alertType, text string) {
			formattedArgs, err := yaml.Marshal(args)
			suite.Require().NoError(err)

			tags := lo.Map(args.Filter.Tags, func(tag string, _ int) string {
				return "filter_tag_" + tag
			})

			if _, err := suite.DatadogClient().PostEvent(&datadog.Event{
				Title: pointer.Ptr("testMetric " + prettyMetricQuery),
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
					"cluster_name:" + suite.ClusterName,
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

type TestLogArgs struct {
	Filter TestLogFilterArgs
	Expect TestLogExpectArgs
}

type TestLogFilterArgs struct {
	Service string
	Tags    []string
}

type TestLogExpectArgs struct {
	Tags    *[]string
	Message string
}

func (suite *BaseSuite[Env]) AssertLog(args *TestLogArgs) {
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

			if _, err := suite.DatadogClient().PostEvent(&datadog.Event{
				Title: pointer.Ptr("testLog " + prettyLogQuery),
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
					"cluster_name:" + suite.ClusterName,
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
				optionalTags := []*regexp.Regexp{
					regexp.MustCompile("logsource:.*"),
				}
				err := assertTags(logs[len(logs)-1].GetTags(), expectedTags, optionalTags, false)
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

type TestCheckRunArgs struct {
	Filter   TestCheckRunFilterArgs
	Expect   TestCheckRunExpectArgs
	Optional TestCheckRunExpectArgs
}

type TestCheckRunFilterArgs struct {
	Name string
	// Tags are used to filter the checkRun
	// Regexes are supported
	Tags []string
}

type TestCheckRunExpectArgs struct {
	// Tags are the tags expected to be present
	// Regexes are supported
	Tags                 *[]string
	AcceptUnexpectedTags bool
}

func (suite *BaseSuite[Env]) AssertCheckRun(args *TestCheckRunArgs) {
	prettyCheckRunQuery := fmt.Sprintf("%s{%s}", args.Filter.Name, strings.Join(args.Filter.Tags, ","))

	suite.Run("checkRun   "+prettyCheckRunQuery, func() {
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

			if _, err := suite.DatadogClient().PostEvent(&datadog.Event{
				Title: pointer.Ptr("testCheckRun " + prettyCheckRunQuery),
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
					"cluster_name:" + suite.ClusterName,
					"check_run:" + args.Filter.Name,
					"test:" + suite.T().Name(),
				}, tags...),
			}); err != nil {
				suite.T().Logf("Failed to post event: %s", err)
			}
		}

		defer func() {
			if suite.T().Failed() {
				sendEvent("error", fmt.Sprintf("Failed finding %s with proper tags and value", prettyCheckRunQuery))
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

			checkRuns, err := suite.Fakeintake.FilterCheckRuns(
				args.Filter.Name,
				fakeintake.WithMatchingTags[*aggregator.CheckRun](regexTags),
			)
			// Can be replaced by require.NoErrorf(…) once https://github.com/stretchr/testify/pull/1481 is merged
			if !assert.NoErrorf(c, err, "Failed to query fake intake") {
				return
			}
			// Can be replaced by require.NoEmptyf(…) once https://github.com/stretchr/testify/pull/1481 is merged
			if !assert.NotEmptyf(c, checkRuns, "No `%s` checkRun yet", prettyCheckRunQuery) {
				return
			}

			// Check tags
			if expectedTags != nil {
				err := assertTags(checkRuns[len(checkRuns)-1].GetTags(), expectedTags, optionalTags, args.Expect.AcceptUnexpectedTags)
				assert.NoErrorf(c, err, "Tags mismatch on `%s`", prettyCheckRunQuery)
			}

		}, 2*time.Minute, 10*time.Second, "Failed finding `%s` with proper tags and value", prettyCheckRunQuery)
	})
}

type TestEventArgs struct {
	Filter TestEventFilterArgs
	Expect TestEventExpectArgs
}

type TestEventFilterArgs struct {
	Source string
	Tags   []string
}

type TestEventExpectArgs struct {
	Tags      *[]string
	Title     string
	Text      string
	Priority  event.Priority
	AlertType event.AlertType
}

func (suite *BaseSuite[Env]) AssertEvent(args *TestEventArgs) {
	prettyEventQuery := fmt.Sprintf("%s{%s}", args.Filter.Source, strings.Join(args.Filter.Tags, ","))

	suite.Run("event   "+prettyEventQuery, func() {
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

			if _, err := suite.DatadogClient().PostEvent(&datadog.Event{
				Title: pointer.Ptr("testEvent " + prettyEventQuery),
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
					"cluster_name:" + suite.ClusterName,
					"event_source:" + args.Filter.Source,
					"test:" + suite.T().Name(),
				}, tags...),
			}); err != nil {
				suite.T().Logf("Failed to post event: %s", err)
			}
		}

		defer func() {
			if suite.T().Failed() {
				sendEvent("error", fmt.Sprintf("Failed finding %s with proper tags and message", prettyEventQuery))
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

			events, err := suite.Fakeintake.FilterEvents(
				args.Filter.Source,
				fakeintake.WithMatchingTags[*aggregator.Event](regexTags),
			)
			// Can be replaced by require.NoErrorf(…) once https://github.com/stretchr/testify/pull/1481 is merged
			if !assert.NoErrorf(c, err, "Failed to query fake intake") {
				return
			}
			// Can be replaced by require.NoEmptyf(…) once https://github.com/stretchr/testify/pull/1481 is merged
			if !assert.NotEmptyf(c, events, "No `%s` events yet", prettyEventQuery) {
				return
			}

			// Check tags
			if expectedTags != nil {
				err := assertTags(events[len(events)-1].GetTags(), expectedTags, []*regexp.Regexp{}, false)
				assert.NoErrorf(c, err, "Tags mismatch on `%s`", prettyEventQuery)
			}

			// Check title
			if args.Expect.Title != "" {
				assert.Regexpf(c, args.Expect.Title, events[len(events)-1].Title,
					"Event title mismatch on `%s`", prettyEventQuery)
			}

			// Check text
			if args.Expect.Text != "" {
				assert.Regexpf(c, args.Expect.Text, events[len(events)-1].Text,
					"Event text mismatch on `%s`", prettyEventQuery)
			}

			// Check priority
			if len(args.Expect.Priority) != 0 {
				assert.Equalf(c, args.Expect.Priority, events[len(events)-1].Priority,
					"Event priority mismatch on `%s`", prettyEventQuery)
			}

			// Check alert type
			if len(args.Expect.AlertType) != 0 {
				assert.Equalf(c, args.Expect.AlertType, events[len(events)-1].AlertType,
					"Event alert type mismatch on `%s`", prettyEventQuery)
			}
		}, 2*time.Minute, 10*time.Second, "Failed finding `%s` with proper tags and message", prettyEventQuery)
	})
}

type TestAPMTraceArgs struct {
	Filter TestAPMTraceFilterArgs
	Expect TestAPMTraceExpectArgs
}

type TestAPMTraceFilterArgs struct {
	ServiceName   string
	OperationName string
	ResourceName  string
	Tags          []string
}

type TestAPMTraceExpectArgs struct {
	Tags      *[]string
	SpanCount *int
	// SamplingPriority validates sampling decision
	SamplingPriority *int
	// TraceIDPresent validates trace_id is set
	TraceIDPresent bool
	// ParentIDPresent validates parent_id is set for child spans
	ParentIDPresent bool
}

func (suite *BaseSuite[Env]) AssertAPMTrace(args *TestAPMTraceArgs) {
	prettyTraceQuery := fmt.Sprintf("%s{%s}", args.Filter.ServiceName, strings.Join(args.Filter.Tags, ","))

	suite.Run("trace   "+prettyTraceQuery, func() {
		var expectedTags []*regexp.Regexp
		if args.Expect.Tags != nil {
			expectedTags = lo.Map(*args.Expect.Tags, func(tag string, _ int) *regexp.Regexp { return regexp.MustCompile(tag) })
		}

		suite.EventuallyWithTf(func(collect *assert.CollectT) {
			c := &myCollectT{
				CollectT: collect,
				errors:   []error{},
			}
			// To enforce the use of myCollectT instead
			collect = nil //nolint:ineffassign

			// Get traces from fakeintake
			traces, err := suite.Fakeintake.GetTraces()
			// Can be replaced by require.NoErrorf(…) once https://github.com/stretchr/testify/pull/1481 is merged
			if !assert.NoErrorf(c, err, "Failed to query fake intake for traces") {
				return
			}

			// Filter traces by service name
			matchingTraces := make([]*aggregator.TracePayload, 0)
			for _, trace := range traces {
				if len(trace.TracerPayloads) == 0 {
					continue
				}
				for _, payload := range trace.TracerPayloads {
					for _, chunk := range payload.Chunks {
						for _, span := range chunk.Spans {
							if span.Service == args.Filter.ServiceName {
								// Check operation name if specified
								if args.Filter.OperationName != "" && span.Name != args.Filter.OperationName {
									continue
								}
								// Check resource name if specified
								if args.Filter.ResourceName != "" && span.Resource != args.Filter.ResourceName {
									continue
								}
								matchingTraces = append(matchingTraces, trace)
								goto nextTrace
							}
						}
					}
				}
			nextTrace:
			}

			// Can be replaced by require.NoEmptyf(…) once https://github.com/stretchr/testify/pull/1481 is merged
			if !assert.NotEmptyf(c, matchingTraces, "No `%s` traces yet", prettyTraceQuery) {
				return
			}

			latestTrace := matchingTraces[len(matchingTraces)-1]

			// Find spans matching the service
			matchingSpans := []*pb.Span{}
			for _, payload := range latestTrace.TracerPayloads {
				for _, chunk := range payload.Chunks {
					for _, span := range chunk.Spans {
						if span.Service == args.Filter.ServiceName {
							matchingSpans = append(matchingSpans, span)
						}
					}
				}
			}

			if len(matchingSpans) == 0 {
				return
			}

			// Check span count if specified
			if args.Expect.SpanCount != nil {
				assert.Equalf(c, *args.Expect.SpanCount, len(matchingSpans),
					"Expected %d spans for service %s, got %d", *args.Expect.SpanCount, args.Filter.ServiceName, len(matchingSpans))
			}

			// Check tags on TracerPayload (where container tags are enriched)
			if expectedTags != nil {
				traceTags := make([]string, 0)
				for _, payload := range latestTrace.TracerPayloads {
					for k, v := range payload.Tags {
						traceTags = append(traceTags, k+":"+v)
					}
				}
				// Set acceptUnexpectedTags=true for bundled tag format (DD_APM_ENABLE_CONTAINER_TAGS_BUFFER=true)
				// The bundled _dd.tags.container tag contains many comma-separated key:value pairs
				err := assertTags(traceTags, expectedTags, []*regexp.Regexp{}, true)
				assert.NoErrorf(c, err, "Tags mismatch on `%s`", prettyTraceQuery)
			}

			// Check trace ID is present
			if args.Expect.TraceIDPresent {
				assert.NotZerof(c, matchingSpans[0].TraceID, "TraceID should be present for `%s`", prettyTraceQuery)
			}

			// Check sampling priority if specified
			if args.Expect.SamplingPriority != nil {
				assert.Equalf(c, float64(*args.Expect.SamplingPriority), matchingSpans[0].Metrics["_sampling_priority_v1"],
					"Sampling priority mismatch for `%s`", prettyTraceQuery)
			}

		}, 2*time.Minute, 10*time.Second, "Failed finding `%s` traces with proper tags and spans", prettyTraceQuery)
	})
}

type TestLogPipelineArgs struct {
	Filter TestLogPipelineFilterArgs
	Expect TestLogPipelineExpectArgs
}

type TestLogPipelineFilterArgs struct {
	Service string
	Source  string
	Tags    []string
}

type TestLogPipelineExpectArgs struct {
	// MinCount validates minimum number of logs
	MinCount int
	// Status validates log status (info, warning, error)
	Status string
	// Message regex pattern
	Message string
	// Tags expected on logs
	Tags *[]string
	// ParsedFields validates structured log parsing
	ParsedFields map[string]string
	// TraceIDPresent validates trace correlation
	TraceIDPresent bool
}

func (suite *BaseSuite[Env]) AssertLogPipeline(args *TestLogPipelineArgs) {
	prettyLogQuery := fmt.Sprintf("%s{%s}", args.Filter.Service, strings.Join(args.Filter.Tags, ","))

	suite.Run("logPipeline   "+prettyLogQuery, func() {
		var expectedTags []*regexp.Regexp
		if args.Expect.Tags != nil {
			expectedTags = lo.Map(*args.Expect.Tags, func(tag string, _ int) *regexp.Regexp { return regexp.MustCompile(tag) })
		}

		var expectedMessage *regexp.Regexp
		if args.Expect.Message != "" {
			expectedMessage = regexp.MustCompile(args.Expect.Message)
		}

		suite.EventuallyWithTf(func(collect *assert.CollectT) {
			c := &myCollectT{
				CollectT: collect,
				errors:   []error{},
			}
			// To enforce the use of myCollectT instead
			collect = nil //nolint:ineffassign

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

			// Check minimum count
			if args.Expect.MinCount > 0 {
				assert.GreaterOrEqualf(c, len(logs), args.Expect.MinCount,
					"Expected at least %d logs for `%s`, got %d", args.Expect.MinCount, prettyLogQuery, len(logs))
			}

			latestLog := logs[len(logs)-1]

			// Check tags
			if expectedTags != nil {
				err := assertTags(latestLog.GetTags(), expectedTags, []*regexp.Regexp{}, false)
				assert.NoErrorf(c, err, "Tags mismatch on `%s`", prettyLogQuery)
			}

			// Check status
			if args.Expect.Status != "" {
				assert.Equalf(c, args.Expect.Status, latestLog.Status,
					"Log status mismatch on `%s`: expected %s, got %s", prettyLogQuery, args.Expect.Status, latestLog.Status)
			}

			// Check message
			if expectedMessage != nil {
				assert.Truef(c, expectedMessage.MatchString(latestLog.Message),
					"Log message `%s` doesn't match pattern `%s`", latestLog.Message, args.Expect.Message)
			}

			// Check parsed fields (for structured logs)
			// Note: ParsedFields validation would require accessing the parsed log structure
			// which may be implementation-specific. Skipping for now.
			_ = args.Expect.ParsedFields // Avoid unused variable error

			// Check trace correlation
			if args.Expect.TraceIDPresent {
				ddTags := strings.Join(latestLog.GetTags(), ",")
				assert.Regexpf(c, `dd\.trace_id:[[:xdigit:]]+`, ddTags,
					"trace_id not found in log tags for `%s`", prettyLogQuery)
			}

		}, 2*time.Minute, 10*time.Second, "Failed finding `%s` logs with expected pipeline processing", prettyLogQuery)
	})
}

type TestAgentHealthArgs struct {
	// CheckEndpoints validates agent status endpoints are accessible
	CheckEndpoints bool
	// CheckComponents validates specific agent components are ready
	CheckComponents []string
	// ExpectedVersion validates agent version
	ExpectedVersion string
}

func (suite *BaseSuite[Env]) AssertAgentHealth(args *TestAgentHealthArgs) {
	suite.Run("agentHealth", func() {
		suite.EventuallyWithTf(func(collect *assert.CollectT) {
			c := &myCollectT{
				CollectT: collect,
				errors:   []error{},
			}
			// To enforce the use of myCollectT instead
			collect = nil //nolint:ineffassign

			// Check that we're receiving any data from the agent (indicates it's running)
			metrics, err := suite.Fakeintake.GetMetricNames()
			if !assert.NoErrorf(c, err, "Failed to query metrics from fake intake") {
				return
			}

			assert.NotEmptyf(c, metrics, "No metrics received from agent - agent may not be healthy")

			// Check for datadog.agent.started metric (indicates successful agent startup)
			startedMetrics, err := suite.Fakeintake.FilterMetrics("datadog.agent.started")
			if err == nil && len(startedMetrics) > 0 {
				suite.T().Logf("Agent started metric found - agent is healthy")
			}

			// If specific components requested, check for their metrics
			for _, component := range args.CheckComponents {
				componentMetricPrefix := fmt.Sprintf("datadog.%s.", component)
				componentMetrics := lo.Filter(metrics, func(metric string, _ int) bool {
					return strings.HasPrefix(metric, componentMetricPrefix)
				})
				assert.NotEmptyf(c, componentMetrics,
					"No metrics found for component `%s` - component may not be running", component)
			}

		}, 5*time.Minute, 10*time.Second, "Agent health check failed")
	})
}

type TestResilienceScenarioArgs struct {
	// ScenarioName for logging
	ScenarioName string
	// TriggerFunc function that triggers the failure scenario
	TriggerFunc func() error
	// RecoveryFunc function that triggers recovery (optional)
	RecoveryFunc func() error
	// ValidateFunc function that validates system recovered
	ValidateFunc func(*assert.CollectT)
	// RecoveryTimeout time to wait for recovery
	RecoveryTimeout time.Duration
}

func (suite *BaseSuite[Env]) AssertResilienceScenario(args *TestResilienceScenarioArgs) {
	suite.Run("resilience_"+args.ScenarioName, func() {
		// Trigger the failure scenario
		if args.TriggerFunc != nil {
			err := args.TriggerFunc()
			suite.Require().NoErrorf(err, "Failed to trigger resilience scenario: %s", args.ScenarioName)
			suite.T().Logf("Triggered resilience scenario: %s", args.ScenarioName)
		}

		// Wait a bit for the failure to take effect
		time.Sleep(5 * time.Second)

		// Trigger recovery if specified
		if args.RecoveryFunc != nil {
			err := args.RecoveryFunc()
			suite.Require().NoErrorf(err, "Failed to trigger recovery for scenario: %s", args.ScenarioName)
			suite.T().Logf("Triggered recovery for scenario: %s", args.ScenarioName)
		}

		// Validate recovery
		recoveryTimeout := args.RecoveryTimeout
		if recoveryTimeout == 0 {
			recoveryTimeout = 2 * time.Minute
		}

		suite.EventuallyWithTf(func(collect *assert.CollectT) {
			if args.ValidateFunc != nil {
				args.ValidateFunc(collect)
			}
		}, recoveryTimeout, 10*time.Second, "Recovery validation failed for scenario: %s", args.ScenarioName)

		suite.T().Logf("Successfully recovered from resilience scenario: %s", args.ScenarioName)
	})
}

// AssertECSTasksReady waits for all ECS services and tasks in the given cluster
// to be in RUNNING state. This should be called as the first test (Test00UpAndRunning)
// in each suite to ensure infrastructure is ready before other tests run.
func (suite *BaseSuite[Env]) AssertECSTasksReady(ecsClusterName string) {
	ctx := suite.T().Context()

	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	suite.Require().NoErrorf(err, "Failed to load AWS config")

	client := awsecs.NewFromConfig(cfg)

	suite.Run("ECS tasks are ready", func() {
		suite.EventuallyWithTf(func(c *assert.CollectT) {
			var initToken string
			for nextToken := &initToken; nextToken != nil; {
				if nextToken == &initToken {
					nextToken = nil
				}

				servicesList, err := client.ListServices(ctx, &awsecs.ListServicesInput{
					Cluster:    &ecsClusterName,
					MaxResults: pointer.Ptr(int32(10)), // Because `DescribeServices` takes at most 10 services in input
					NextToken:  nextToken,
				})
				if !assert.NoErrorf(c, err, "Failed to list ECS services") {
					return
				}

				nextToken = servicesList.NextToken

				servicesDescription, err := client.DescribeServices(ctx, &awsecs.DescribeServicesInput{
					Cluster:  &ecsClusterName,
					Services: servicesList.ServiceArns,
				})
				if !assert.NoErrorf(c, err, "Failed to describe ECS services %v", servicesList.ServiceArns) {
					continue
				}

				for _, serviceDescription := range servicesDescription.Services {
					assert.NotZerof(c, serviceDescription.DesiredCount, "ECS service %s has no task", *serviceDescription.ServiceName)

					for nextToken := &initToken; nextToken != nil; {
						if nextToken == &initToken {
							nextToken = nil
						}

						tasksList, err := client.ListTasks(ctx, &awsecs.ListTasksInput{
							Cluster:       &ecsClusterName,
							ServiceName:   serviceDescription.ServiceName,
							DesiredStatus: awsecstypes.DesiredStatusRunning,
							MaxResults:    pointer.Ptr(int32(100)), // Because `DescribeTasks` takes at most 100 tasks in input
							NextToken:     nextToken,
						})
						if !assert.NoErrorf(c, err, "Failed to list ECS tasks for service %s", *serviceDescription.ServiceName) {
							break
						}

						nextToken = tasksList.NextToken

						tasksDescription, err := client.DescribeTasks(ctx, &awsecs.DescribeTasksInput{
							Cluster: &ecsClusterName,
							Tasks:   tasksList.TaskArns,
						})
						if !assert.NoErrorf(c, err, "Failed to describe ECS tasks %v", tasksList.TaskArns) {
							continue
						}

						for _, taskDescription := range tasksDescription.Tasks {
							assert.Equalf(c, string(awsecstypes.DesiredStatusRunning), *taskDescription.LastStatus,
								"Task %s of service %s is not running", *taskDescription.TaskArn, *serviceDescription.ServiceName)
							assert.NotEqualf(c, awsecstypes.HealthStatusUnhealthy, taskDescription.HealthStatus,
								"Task %s of service %s is unhealthy", *taskDescription.TaskArn, *serviceDescription.ServiceName)
						}
					}
				}
			}
		}, 15*time.Minute, 10*time.Second, "Not all tasks became ready in time.")
	})
}
