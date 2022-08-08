// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serverless/executioncontext"
	serverlessMetrics "github.com/DataDog/datadog-agent/pkg/serverless/metrics"
)

func TestUnmarshalExtensionLog(t *testing.T) {
	raw, err := ioutil.ReadFile("./testdata/extension_log.json")
	require.NoError(t, err)
	var messages []logMessage
	err = json.Unmarshal(raw, &messages)
	require.NoError(t, err)

	expectedTime, _ := time.Parse(logMessageTimeLayout, "2020-08-20T12:31:32.123Z")
	expectedLogMessage := logMessage{
		logType:      logTypeExtension,
		time:         expectedTime,
		stringRecord: "sample extension log",
	}
	assert.Equal(t, expectedLogMessage, messages[0])
}

func TestShouldProcessLog(t *testing.T) {

	validLog := &logMessage{
		logType: logTypePlatformReport,
		objectRecord: platformObjectRecord{
			requestID: "8286a188-ba32-4475-8077-530cd35c09a9",
		},
	}

	invalidLog0 := &logMessage{
		logType: logTypePlatformLogsSubscription,
	}

	invalidLog1 := &logMessage{
		logType: logTypePlatformExtension,
	}

	nonEmptyARN := "arn:aws:lambda:us-east-1:123456789012:function:my-function"
	emptyARN := ""

	nonEmptyRequestID := "8286a188-ba32-4475-8077-530cd35c09a9"
	emptyRequestID := ""

	assert.True(t, shouldProcessLog(&executioncontext.State{ARN: nonEmptyARN, LastRequestID: nonEmptyRequestID}, validLog))
	assert.False(t, shouldProcessLog(&executioncontext.State{ARN: emptyARN, LastRequestID: emptyRequestID}, validLog))
	assert.False(t, shouldProcessLog(&executioncontext.State{ARN: nonEmptyARN, LastRequestID: emptyRequestID}, validLog))
	assert.False(t, shouldProcessLog(&executioncontext.State{ARN: emptyARN, LastRequestID: nonEmptyRequestID}, validLog))

	assert.False(t, shouldProcessLog(&executioncontext.State{ARN: nonEmptyARN, LastRequestID: nonEmptyRequestID}, invalidLog0))
	assert.False(t, shouldProcessLog(&executioncontext.State{ARN: emptyARN, LastRequestID: emptyRequestID}, invalidLog0))
	assert.False(t, shouldProcessLog(&executioncontext.State{ARN: nonEmptyARN, LastRequestID: emptyRequestID}, invalidLog0))
	assert.False(t, shouldProcessLog(&executioncontext.State{ARN: emptyARN, LastRequestID: nonEmptyRequestID}, invalidLog0))

	assert.False(t, shouldProcessLog(&executioncontext.State{ARN: nonEmptyARN, LastRequestID: nonEmptyRequestID}, invalidLog1))
	assert.False(t, shouldProcessLog(&executioncontext.State{ARN: emptyARN, LastRequestID: emptyRequestID}, invalidLog1))
	assert.False(t, shouldProcessLog(&executioncontext.State{ARN: nonEmptyARN, LastRequestID: emptyRequestID}, invalidLog1))
	assert.False(t, shouldProcessLog(&executioncontext.State{ARN: emptyARN, LastRequestID: nonEmptyRequestID}, invalidLog1))
}

func TestShouldNotProcessEmptyLog(t *testing.T) {
	assert.True(t, shouldProcessLog(
		&executioncontext.State{
			ARN:           "arn:aws:lambda:us-east-1:123456789012:function:my-function",
			LastRequestID: "8286a188-ba32-4475-8077-530cd35c09a9",
		},
		&logMessage{
			stringRecord: "aaa",
			objectRecord: platformObjectRecord{
				requestID: "",
			},
		}),
	)
	assert.True(t, shouldProcessLog(
		&executioncontext.State{
			ARN:           "arn:aws:lambda:us-east-1:123456789012:function:my-function",
			LastRequestID: "8286a188-ba32-4475-8077-530cd35c09a9",
		},
		&logMessage{
			stringRecord: "",
			objectRecord: platformObjectRecord{
				requestID: "aaa",
			},
		}),
	)
	assert.False(t, shouldProcessLog(
		&executioncontext.State{
			ARN:           "arn:aws:lambda:us-east-1:123456789012:function:my-function",
			LastRequestID: "8286a188-ba32-4475-8077-530cd35c09a9",
		},
		&logMessage{
			stringRecord: "",
			objectRecord: platformObjectRecord{
				requestID: "",
			},
		}),
	)
}
func TestCreateStringRecordForReportLogWithInitDuration(t *testing.T) {
	var sampleLogMessage = &logMessage{
		objectRecord: platformObjectRecord{
			requestID: "cf84ebaf-606a-4b0f-b99b-3685bfe973d7",
			reportLogItem: reportLogMetrics{
				durationMs:       100.00,
				billedDurationMs: 100,
				memorySizeMB:     128,
				maxMemoryUsedMB:  128,
				initDurationMs:   50.00,
			},
		},
	}

	now := time.Now()
	ecs := executioncontext.State{
		StartTime: now,
		EndTime:   now.Add(10 * time.Millisecond),
	}

	output := createStringRecordForReportLog(sampleLogMessage, ecs)
	expectedOutput := "REPORT RequestId: cf84ebaf-606a-4b0f-b99b-3685bfe973d7\tDuration: 100.00 ms\tRuntime Duration: 10.00 ms\tPost Runtime Duration: 90.00 ms\tBilled Duration: 100 ms\tMemory Size: 128 MB\tMax Memory Used: 128 MB\tInit Duration: 50.00 ms"
	assert.Equal(t, expectedOutput, output)
}

func TestCreateStringRecordForReportLogWithoutInitDuration(t *testing.T) {
	var sampleLogMessage = &logMessage{
		objectRecord: platformObjectRecord{
			requestID: "cf84ebaf-606a-4b0f-b99b-3685bfe973d7",
			reportLogItem: reportLogMetrics{
				durationMs:       100.00,
				billedDurationMs: 100,
				memorySizeMB:     128,
				maxMemoryUsedMB:  128,
				initDurationMs:   0.00,
			},
		},
	}

	now := time.Now()
	ecs := executioncontext.State{
		StartTime: now,
		EndTime:   now.Add(10 * time.Millisecond),
	}

	output := createStringRecordForReportLog(sampleLogMessage, ecs)
	expectedOutput := "REPORT RequestId: cf84ebaf-606a-4b0f-b99b-3685bfe973d7\tDuration: 100.00 ms\tRuntime Duration: 10.00 ms\tPost Runtime Duration: 90.00 ms\tBilled Duration: 100 ms\tMemory Size: 128 MB\tMax Memory Used: 128 MB"
	assert.Equal(t, expectedOutput, output)
}

func TestRemoveInvalidTracingItemWellFormatted(t *testing.T) {
	raw, err := ioutil.ReadFile("./testdata/valid_logs_payload.json")
	require.NoError(t, err)
	sanitizedData := removeInvalidTracingItem(raw)
	assert.Equal(t, raw, sanitizedData)
}

func TestRemoveInvalidTracingItemNotWellFormatted(t *testing.T) {
	raw, err := ioutil.ReadFile("./testdata/invalid_logs_payload.json")
	require.NoError(t, err)
	sanitizedData := removeInvalidTracingItem(raw)
	sanitizedRaw, sanitizedErr := ioutil.ReadFile("./testdata/invalid_logs_payload_sanitized.json")
	require.NoError(t, sanitizedErr)
	assert.Equal(t, string(sanitizedRaw), string(sanitizedData))
}

func TestParseLogsAPIPayloadWellFormated(t *testing.T) {
	raw, err := ioutil.ReadFile("./testdata/valid_logs_payload.json")
	require.NoError(t, err)
	messages, err := parseLogsAPIPayload(raw)
	assert.Nil(t, err)
	assert.NotNil(t, messages)
	assert.NotNil(t, messages[0].objectRecord.requestID)
}

func TestParseLogsAPIPayloadNotWellFormated(t *testing.T) {
	raw, err := ioutil.ReadFile("./testdata/invalid_logs_payload.json")
	require.NoError(t, err)
	messages, err := parseLogsAPIPayload(raw)
	assert.Nil(t, err)
	assert.NotNil(t, messages[0].objectRecord.requestID)
}

func TestParseLogsAPIPayloadNotWellFormatedButNotRecoverable(t *testing.T) {
	raw, err := ioutil.ReadFile("./testdata/invalid_logs_payload_unrecoverable.json")
	require.NoError(t, err)
	_, err = parseLogsAPIPayload(raw)
	assert.NotNil(t, err)
}

func TestProcessMessageValid(t *testing.T) {
	demux := aggregator.InitTestAgentDemultiplexerWithFlushInterval(time.Hour)
	defer demux.Stop(false)

	message := logMessage{
		logType: logTypePlatformReport,
		time:    time.Now(),
		objectRecord: platformObjectRecord{
			reportLogItem: reportLogMetrics{
				durationMs:       1000.0,
				billedDurationMs: 800.0,
				memorySizeMB:     1024.0,
				maxMemoryUsedMB:  256.0,
				initDurationMs:   100.0,
			},
			requestID: "8286a188-ba32-4475-8077-530cd35c09a9",
		},
	}
	arn := "arn:aws:lambda:us-east-1:123456789012:function:test-function"
	lastRequestID := "8286a188-ba32-4475-8077-530cd35c09a9"
	metricTags := []string{"functionname:test-function"}

	computeEnhancedMetrics := true
	mockExecutionContext := &executioncontext.ExecutionContext{}
	mockExecutionContext.SetFromInvocation(arn, lastRequestID)

	processMessage(&message, mockExecutionContext, computeEnhancedMetrics, metricTags, demux, func() {})

	received, timed := demux.WaitForSamples(100 * time.Millisecond)
	assert.Len(t, received, 7)
	assert.Len(t, timed, 0)
	demux.Reset()

	computeEnhancedMetrics = false
	processMessage(&message, mockExecutionContext, computeEnhancedMetrics, metricTags, demux, func() {})

	received, timed = demux.WaitForSamples(100 * time.Millisecond)
	assert.Len(t, received, 0, "we should NOT have received metrics")
	assert.Len(t, timed, 0)
}

func TestProcessMessageStartValid(t *testing.T) {
	demux := aggregator.InitTestAgentDemultiplexerWithFlushInterval(time.Hour)
	defer demux.Stop(false)

	message := &logMessage{
		logType: logTypePlatformStart,
		time:    time.Now(),
		objectRecord: platformObjectRecord{
			requestID: "8286a188-ba32-4475-8077-530cd35c09a9",
		},
	}
	arn := "arn:aws:lambda:us-east-1:123456789012:function:test-function"
	lastRequestID := "8286a188-ba32-4475-8077-530cd35c09a9"
	metricTags := []string{"functionname:test-function"}

	mockExecutionContext := &executioncontext.ExecutionContext{}
	mockExecutionContext.SetFromInvocation(arn, lastRequestID)

	computeEnhancedMetrics := true

	runtimeDoneCallbackWasCalled := false
	mockRuntimeDone := func() {
		runtimeDoneCallbackWasCalled = true
	}

	processMessage(message, mockExecutionContext, computeEnhancedMetrics, metricTags, demux, mockRuntimeDone)
	ecs := mockExecutionContext.GetCurrentState()
	assert.Equal(t, lastRequestID, ecs.LastLogRequestID)
	assert.Equal(t, runtimeDoneCallbackWasCalled, false)
}

func TestProcessMessagePlatformRuntimeDoneValid(t *testing.T) {
	demux := aggregator.InitTestAgentDemultiplexerWithFlushInterval(time.Hour)
	messageTime := time.Now()
	defer demux.Stop(false)
	message := logMessage{
		logType: logTypePlatformRuntimeDone,
		time:    messageTime,
		objectRecord: platformObjectRecord{
			requestID: "8286a188-ba32-4475-8077-530cd35c09a9",
			runtimeDoneItem: runtimeDoneItem{
				status: "success",
			},
		},
	}
	arn := "arn:aws:lambda:us-east-1:123456789012:function:test-function"
	lastRequestID := "8286a188-ba32-4475-8077-530cd35c09a9"
	metricTags := []string{"functionname:test-function"}
	computeEnhancedMetrics := true

	mockExecutionContext := &executioncontext.ExecutionContext{}
	mockExecutionContext.SetFromInvocation(arn, lastRequestID)

	runtimeDoneCallbackWasCalled := false
	mockRuntimeDone := func() {
		runtimeDoneCallbackWasCalled = true
	}

	processMessage(&message, mockExecutionContext, computeEnhancedMetrics, metricTags, demux, mockRuntimeDone)
	ecs := mockExecutionContext.GetCurrentState()
	assert.Equal(t, runtimeDoneCallbackWasCalled, true)
	assert.WithinDuration(t, messageTime, ecs.EndTime, time.Millisecond)
}

func TestProcessMessagePlatformRuntimeDonePreviousInvocation(t *testing.T) {
	demux := aggregator.InitTestAgentDemultiplexerWithFlushInterval(time.Hour)
	defer demux.Stop(false)

	previousRequestID := "9397b299-cb43-5586-9188-641de46d10b0"
	currentRequestID := "8286a188-ba32-4475-8077-530cd35c09a9"
	message := &logMessage{
		logType: logTypePlatformRuntimeDone,
		time:    time.Now(),
		objectRecord: platformObjectRecord{
			requestID: previousRequestID,
			runtimeDoneItem: runtimeDoneItem{
				status: "success",
			},
		},
	}
	arn := "arn:aws:lambda:us-east-1:123456789012:function:test-function"
	lastRequestID := currentRequestID
	metricTags := []string{"functionname:test-function"}

	computeEnhancedMetrics := true
	mockExecutionContext := &executioncontext.ExecutionContext{}
	mockExecutionContext.SetFromInvocation(arn, lastRequestID)

	runtimeDoneCallbackWasCalled := false
	mockRuntimeDone := func() {
		runtimeDoneCallbackWasCalled = true
	}

	processMessage(message, mockExecutionContext, computeEnhancedMetrics, metricTags, demux, mockRuntimeDone)
	// Runtime done callback should NOT be called if the log message was for a previous invocation
	assert.Equal(t, runtimeDoneCallbackWasCalled, false)
}

func TestProcessMessageShouldNotProcessArnNotSet(t *testing.T) {
	demux := aggregator.InitTestAgentDemultiplexerWithFlushInterval(time.Hour)
	defer demux.Stop(false)
	message := &logMessage{
		logType: logTypePlatformReport,
		time:    time.Now(),
		objectRecord: platformObjectRecord{
			reportLogItem: reportLogMetrics{
				durationMs:       1000.0,
				billedDurationMs: 800.0,
				memorySizeMB:     1024.0,
				maxMemoryUsedMB:  256.0,
				initDurationMs:   100.0,
			},
		},
	}

	metricTags := []string{"functionname:test-function"}

	mockExecutionContext := &executioncontext.ExecutionContext{}

	computeEnhancedMetrics := true
	go processMessage(message, mockExecutionContext, computeEnhancedMetrics, metricTags, demux, func() {})

	received, timed := demux.WaitForSamples(100 * time.Millisecond)
	assert.Len(t, received, 0, "We should NOT have received metrics")
	assert.Len(t, timed, 0)
}

func TestProcessMessageShouldNotProcessLogsDropped(t *testing.T) {
	demux := aggregator.InitTestAgentDemultiplexerWithFlushInterval(time.Hour)
	defer demux.Stop(false)
	message := &logMessage{
		logType:      logTypePlatformLogsDropped,
		time:         time.Now(),
		stringRecord: "bla bla bla",
	}

	arn := "arn:aws:lambda:us-east-1:123456789012:function:test-function"
	lastRequestID := "8286a188-ba32-4475-8077-530cd35c09a9"
	metricTags := []string{"functionname:test-function"}
	computeEnhancedMetrics := true

	mockExecutionContext := &executioncontext.ExecutionContext{}
	mockExecutionContext.SetFromInvocation(arn, lastRequestID)

	go processMessage(message, mockExecutionContext, computeEnhancedMetrics, metricTags, demux, func() {})

	received, timed := demux.WaitForSamples(100 * time.Millisecond)
	assert.Len(t, received, 0, "We should NOT have received metrics")
	assert.Len(t, timed, 0)
}

func TestProcessMessageShouldProcessLogTypeFunction(t *testing.T) {
	demux := aggregator.InitTestAgentDemultiplexerWithFlushInterval(time.Hour)
	defer demux.Stop(false)
	message := &logMessage{
		logType:      logTypeFunction,
		time:         time.Now(),
		stringRecord: "fatal error: runtime: out of memory",
	}

	arn := "arn:aws:lambda:us-east-1:123456789012:function:test-function"
	lastRequestID := "8286a188-ba32-4475-8077-530cd35c09a9"
	metricTags := []string{"functionname:test-function"}
	computeEnhancedMetrics := true

	mockExecutionContext := &executioncontext.ExecutionContext{}
	mockExecutionContext.SetFromInvocation(arn, lastRequestID)

	go processMessage(message, mockExecutionContext, computeEnhancedMetrics, metricTags, demux, func() {})

	received, timed := demux.WaitForSamples(100 * time.Millisecond)
	assert.Len(t, received, 2)
	assert.Len(t, timed, 0)
	assert.Equal(t, serverlessMetrics.OutOfMemoryMetric, received[0].Name)
	assert.Equal(t, serverlessMetrics.ErrorsMetric, received[1].Name)
}

func TestProcessLogMessageLogsEnabled(t *testing.T) {

	logChannel := make(chan *config.ChannelMessage)

	mockExecutionContext := &executioncontext.ExecutionContext{}
	mockExecutionContext.SetFromInvocation("my-arn", "myRequestID")

	logCollection := &LambdaLogsCollector{
		LogsEnabled: true,
		LogChannel:  logChannel,
		ExtraTags: &Tags{
			Tags: []string{"tag0:value0,tag1:value1"},
		},
		ExecutionContext: mockExecutionContext,
	}

	logMessages := []logMessage{
		{
			stringRecord: "hi, log 0",
		},
		{
			stringRecord: "hi, log 1",
		},
		{
			stringRecord: "hi, log 2",
		},
	}
	go processLogMessages(logCollection, logMessages)

	select {
	case received := <-logChannel:
		assert.NotNil(t, received)
		assert.Equal(t, "my-arn", received.Lambda.ARN)
		assert.Equal(t, "myRequestID", received.Lambda.RequestID)
	case <-time.After(100 * time.Millisecond):
		assert.Fail(t, "We should have received logs")
	}
}

func TestProcessLogMessageNoStringRecordPlatformLog(t *testing.T) {

	logChannel := make(chan *config.ChannelMessage)

	mockExecutionContext := &executioncontext.ExecutionContext{}
	mockExecutionContext.SetFromInvocation("my-arn", "myRequestID")
	logCollection := &LambdaLogsCollector{
		LogsEnabled: true,
		LogChannel:  logChannel,
		ExtraTags: &Tags{
			Tags: []string{"tag0:value0,tag1:value1"},
		},
		ExecutionContext: mockExecutionContext,
	}

	logMessages := []logMessage{
		{
			logType: logTypePlatformRuntimeDone,
		},
	}
	go processLogMessages(logCollection, logMessages)

	select {
	case <-logChannel:
		assert.Fail(t, "We should not have received logs")
	case <-time.After(100 * time.Millisecond):
		// nothing to do here
	}
}

func TestProcessLogMessageNoStringRecordFunctionLog(t *testing.T) {

	logChannel := make(chan *config.ChannelMessage)

	mockExecutionContext := &executioncontext.ExecutionContext{}
	mockExecutionContext.SetFromInvocation("my-arn", "myRequestID")

	logCollection := &LambdaLogsCollector{
		LogsEnabled: true,
		LogChannel:  logChannel,
		ExtraTags: &Tags{
			Tags: []string{"tag0:value0,tag1:value1"},
		},
		ExecutionContext: mockExecutionContext,
	}

	logMessages := []logMessage{
		{
			stringRecord: "hi, log 2",
		},
	}
	go processLogMessages(logCollection, logMessages)

	select {
	case received := <-logChannel:
		assert.NotNil(t, received)
		assert.Equal(t, "my-arn", received.Lambda.ARN)
		assert.Equal(t, "myRequestID", received.Lambda.RequestID)
	case <-time.After(100 * time.Millisecond):
		assert.Fail(t, "We should have received logs")
	}
}

func TestProcessLogMessageLogsNotEnabled(t *testing.T) {

	logChannel := make(chan *config.ChannelMessage)

	mockExecutionContext := &executioncontext.ExecutionContext{}
	mockExecutionContext.SetFromInvocation("my-arn", "myRequestID")

	logCollection := &LambdaLogsCollector{
		LogsEnabled: false,
		LogChannel:  logChannel,
		ExtraTags: &Tags{
			Tags: []string{"tag0:value0,tag1:value1"},
		},
		ExecutionContext: mockExecutionContext,
	}

	logMessages := []logMessage{
		{
			stringRecord: "hi, log 0",
		},
		{
			stringRecord: "hi, log 1",
		},
		{
			stringRecord: "hi, log 2",
		},
	}
	go processLogMessages(logCollection, logMessages)

	select {
	case <-logChannel:
		assert.Fail(t, "We should not have received logs")
	case <-time.After(100 * time.Millisecond):
		// nothing to do here
	}
}

func TestServeHTTPInvalidPayload(t *testing.T) {
	logChannel := make(chan *config.ChannelMessage)

	mockExecutionContext := &executioncontext.ExecutionContext{}
	mockExecutionContext.SetFromInvocation("my-arn", "myRequestID")

	logCollection := &LambdaLogsCollector{
		LogsEnabled: false,
		LogChannel:  logChannel,
		ExtraTags: &Tags{
			Tags: []string{"tag0:value0,tag1:value1"},
		},
		ExecutionContext: mockExecutionContext,
	}

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	res := httptest.NewRecorder()

	logCollection.ServeHTTP(res, req)
	assert.Equal(t, 400, res.Code)
}

func TestServeHTTPSuccess(t *testing.T) {
	logChannel := make(chan *config.ChannelMessage)

	mockExecutionContext := &executioncontext.ExecutionContext{}
	mockExecutionContext.SetFromInvocation("my-arn", "myRequestID")

	logCollection := &LambdaLogsCollector{
		LogsEnabled: false,
		LogChannel:  logChannel,
		ExtraTags: &Tags{
			Tags: []string{"tag0:value0,tag1:value1"},
		},
		ExecutionContext: mockExecutionContext,
	}

	raw, err := ioutil.ReadFile("./testdata/extension_log.json")
	if err != nil {
		assert.Fail(t, "should be able to read the log file")
	}
	if err != nil {
		assert.Fail(t, "should be able to marshal")
	}
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(raw))
	res := httptest.NewRecorder()

	logCollection.ServeHTTP(res, req)
	assert.Equal(t, 200, res.Code)
}

func TestUnmarshalJSONInvalid(t *testing.T) {
	logMessage := &logMessage{}
	err := logMessage.UnmarshalJSON([]byte("invalid"))
	assert.NotNil(t, err)
}

func TestUnmarshalJSONMalformed(t *testing.T) {
	logMessage := &logMessage{}
	raw, errReadFile := ioutil.ReadFile("./testdata/invalid_log_no_type.json")
	if errReadFile != nil {
		assert.Fail(t, "should be able to read the file")
	}
	err := logMessage.UnmarshalJSON(raw)
	assert.NotNil(t, err)
}

func TestUnmarshalJSONLogTypePlatformLogsSubscription(t *testing.T) {
	logMessage := &logMessage{}
	raw, errReadFile := ioutil.ReadFile("./testdata/platform_log.json")
	if errReadFile != nil {
		assert.Fail(t, "should be able to read the file")
	}
	err := logMessage.UnmarshalJSON(raw)
	assert.Nil(t, err)
	assert.Equal(t, "platform.logsSubscription", logMessage.logType)
}

func TestUnmarshalJSONLogTypePlatformStart(t *testing.T) {
	logMessage := &logMessage{}
	raw, errReadFile := ioutil.ReadFile("./testdata/platform_start.json")
	if errReadFile != nil {
		assert.Fail(t, "should be able to read the file")
	}
	err := logMessage.UnmarshalJSON(raw)
	assert.Nil(t, err)
	assert.Equal(t, "platform.start", logMessage.logType)
	assert.Equal(t, "START RequestId: 13dee504-0d50-4c86-8d82-efd20693afc9 Version: 10", logMessage.stringRecord)
}

func TestUnmarshalJSONLogTypePlatformEnd(t *testing.T) {
	logMessage := &logMessage{}
	raw, errReadFile := ioutil.ReadFile("./testdata/platform_end.json")
	if errReadFile != nil {
		assert.Fail(t, "should be able to read the file")
	}
	err := logMessage.UnmarshalJSON(raw)
	assert.Nil(t, err)
	assert.Equal(t, "platform.end", logMessage.logType)
	assert.Equal(t, "END RequestId: 13dee504-0d50-4c86-8d82-efd20693afc9", logMessage.stringRecord)
}

func TestUnmarshalJSONLogTypeIncorrectReportNotFatalMetrics(t *testing.T) {
	logMessage := &logMessage{}
	raw, errReadFile := ioutil.ReadFile("./testdata/platform_incorrect_report.json")
	if errReadFile != nil {
		assert.Fail(t, "should be able to read the file")
	}
	err := logMessage.UnmarshalJSON(raw)
	assert.Nil(t, err)
}

func TestUnmarshalJSONLogTypeIncorrectReportNotFatalReport(t *testing.T) {
	logMessage := &logMessage{}
	raw, errReadFile := ioutil.ReadFile("./testdata/platform_incorrect_report_record.json")
	if errReadFile != nil {
		assert.Fail(t, "should be able to read the file")
	}
	err := logMessage.UnmarshalJSON(raw)
	assert.Nil(t, err)
}

func TestUnmarshalPlatformRuntimeDoneLog(t *testing.T) {
	raw, err := ioutil.ReadFile("./testdata/platform_runtime_done_log_valid.json")
	require.NoError(t, err)
	var message logMessage
	err = json.Unmarshal(raw, &message)
	require.NoError(t, err)

	expectedTime := time.Date(2021, 05, 19, 18, 11, 22, 478000000, time.UTC)

	expectedLogMessage := logMessage{
		logType: logTypePlatformRuntimeDone,
		time:    expectedTime,
		objectRecord: platformObjectRecord{
			requestID: "13dee504-0d50-4c86-8d82-efd20693afc9",
			runtimeDoneItem: runtimeDoneItem{
				status: "success",
			},
		},
	}
	assert.Equal(t, expectedLogMessage, message)
}

func TestUnmarshalPlatformRuntimeDoneLogNotFatal(t *testing.T) {
	logMessage := &logMessage{}
	raw, errReadFile := ioutil.ReadFile("./testdata/platform_incorrect_runtime_done_log.json")
	if errReadFile != nil {
		assert.Fail(t, "should be able to read the file")
	}
	err := logMessage.UnmarshalJSON(raw)
	assert.Nil(t, err)
}

func TestRuntimeMetricsMatchLogs(t *testing.T) {
	// The test ensures that the values listed in the report log statement
	// matches the values of the metrics being reported.
	demux := aggregator.InitTestAgentDemultiplexerWithFlushInterval(time.Hour)
	defer demux.Stop(false)

	runtimeDurationMs := 10.0
	postRuntimeDurationMs := 90.0
	durationMs := runtimeDurationMs + postRuntimeDurationMs

	startTime := time.Now()
	endTime := startTime.Add(time.Duration(runtimeDurationMs) * time.Millisecond)
	reportLogTime := endTime.Add(time.Duration(postRuntimeDurationMs) * time.Millisecond)

	requestID := "13dee504-0d50-4c86-8d82-efd20693afc9"
	mockExecutionContext := &executioncontext.ExecutionContext{}
	mockExecutionContext.SetFromInvocation("arn not used", requestID)
	mockExecutionContext.UpdateFromStartLog(requestID, startTime)
	computeEnhancedMetrics := true

	doneMessage := &logMessage{
		time:    endTime,
		logType: logTypePlatformRuntimeDone,
		objectRecord: platformObjectRecord{
			requestID:       requestID,
			runtimeDoneItem: runtimeDoneItem{},
		},
	}
	reportMessage := &logMessage{
		time:    reportLogTime,
		logType: logTypePlatformReport,
		objectRecord: platformObjectRecord{
			requestID: requestID,
			reportLogItem: reportLogMetrics{
				durationMs: durationMs,
			},
		},
	}

	processMessage(doneMessage, mockExecutionContext, computeEnhancedMetrics, []string{}, demux, func() {})
	processMessage(reportMessage, mockExecutionContext, computeEnhancedMetrics, []string{}, demux, func() {})

	generatedMetrics, timedMetrics := demux.WaitForSamples(100 * time.Millisecond)
	postRuntimeMetricTimestamp := float64(reportLogTime.UnixNano()) / float64(time.Second)
	runtimeMetricTimestamp := float64(endTime.UnixNano()) / float64(time.Second)
	assert.Equal(t, generatedMetrics[0], metrics.MetricSample{
		Name:       "aws.lambda.enhanced.runtime_duration",
		Value:      runtimeDurationMs, // in milliseconds
		Mtype:      metrics.DistributionType,
		Tags:       []string{"cold_start:true"},
		SampleRate: 1,
		Timestamp:  runtimeMetricTimestamp,
	})
	assert.Equal(t, generatedMetrics[4], metrics.MetricSample{
		Name:       "aws.lambda.enhanced.duration",
		Value:      durationMs / 1000, // in seconds
		Mtype:      metrics.DistributionType,
		Tags:       []string{"cold_start:true"},
		SampleRate: 1,
		Timestamp:  postRuntimeMetricTimestamp,
	})
	assert.Equal(t, generatedMetrics[6], metrics.MetricSample{
		Name:       "aws.lambda.enhanced.post_runtime_duration",
		Value:      postRuntimeDurationMs, // in milliseconds
		Mtype:      metrics.DistributionType,
		Tags:       []string{"cold_start:true"},
		SampleRate: 1,
		Timestamp:  postRuntimeMetricTimestamp,
	})
	expectedStringRecord := fmt.Sprintf("REPORT RequestId: 13dee504-0d50-4c86-8d82-efd20693afc9\tDuration: %.2f ms\tRuntime Duration: %.2f ms\tPost Runtime Duration: %.2f ms\tBilled Duration: 0 ms\tMemory Size: 0 MB\tMax Memory Used: 0 MB", durationMs, runtimeDurationMs, postRuntimeDurationMs)
	assert.Equal(t, reportMessage.stringRecord, expectedStringRecord)
	assert.Len(t, timedMetrics, 0)
}
