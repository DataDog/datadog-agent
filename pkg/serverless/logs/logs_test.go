// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
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
	raw, err := os.ReadFile("./testdata/extension_log.json")
	require.NoError(t, err)
	var messages []LambdaLogAPIMessage
	err = json.Unmarshal(raw, &messages)
	require.NoError(t, err)

	expectedTime, _ := time.Parse(logMessageTimeLayout, "2020-08-20T12:31:32.123Z")
	expectedLogMessage := LambdaLogAPIMessage{
		logType:      logTypeExtension,
		time:         expectedTime,
		stringRecord: "sample extension log",
	}
	assert.Equal(t, expectedLogMessage, messages[0])
}

func TestShouldProcessLog(t *testing.T) {

	validLog := &LambdaLogAPIMessage{
		logType: logTypePlatformReport,
		objectRecord: platformObjectRecord{
			requestID: "8286a188-ba32-4475-8077-530cd35c09a9",
		},
	}

	invalidLog0 := &LambdaLogAPIMessage{
		logType: "platform.logsSubscription",
	}

	invalidLog1 := &LambdaLogAPIMessage{
		logType: "platform.extension",
	}

	assert.True(t, shouldProcessLog(validLog))
	assert.False(t, shouldProcessLog(invalidLog0))
	assert.False(t, shouldProcessLog(invalidLog1))

}

func TestShouldNotProcessEmptyLog(t *testing.T) {
	assert.True(t, shouldProcessLog(

		&LambdaLogAPIMessage{
			stringRecord: "aaa",
			objectRecord: platformObjectRecord{
				requestID: "",
			},
		}),
	)
	assert.True(t, shouldProcessLog(

		&LambdaLogAPIMessage{
			stringRecord: "",
			objectRecord: platformObjectRecord{
				requestID: "aaa",
			},
		}),
	)
	assert.False(t, shouldProcessLog(

		&LambdaLogAPIMessage{
			stringRecord: "",
			objectRecord: platformObjectRecord{
				requestID: "",
			},
		}),
	)
}
func TestCreateStringRecordForReportLogWithInitDuration(t *testing.T) {
	var sampleLogMessage = &LambdaLogAPIMessage{
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

	output := createStringRecordForReportLog(ecs.StartTime, ecs.EndTime, sampleLogMessage)
	expectedOutput := "REPORT RequestId: cf84ebaf-606a-4b0f-b99b-3685bfe973d7\tDuration: 100.00 ms\tRuntime Duration: 10.00 ms\tPost Runtime Duration: 90.00 ms\tBilled Duration: 100 ms\tMemory Size: 128 MB\tMax Memory Used: 128 MB\tInit Duration: 50.00 ms"
	assert.Equal(t, expectedOutput, output)
}

func TestCreateStringRecordForReportLogWithoutInitDuration(t *testing.T) {
	var sampleLogMessage = &LambdaLogAPIMessage{
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

	output := createStringRecordForReportLog(ecs.StartTime, ecs.EndTime, sampleLogMessage)
	expectedOutput := "REPORT RequestId: cf84ebaf-606a-4b0f-b99b-3685bfe973d7\tDuration: 100.00 ms\tRuntime Duration: 10.00 ms\tPost Runtime Duration: 90.00 ms\tBilled Duration: 100 ms\tMemory Size: 128 MB\tMax Memory Used: 128 MB"
	assert.Equal(t, expectedOutput, output)
}

func TestRemoveInvalidTracingItemWellFormatted(t *testing.T) {
	raw, err := os.ReadFile("./testdata/valid_logs_payload.json")
	require.NoError(t, err)
	sanitizedData := removeInvalidTracingItem(raw)
	assert.Equal(t, raw, sanitizedData)
}

func TestRemoveInvalidTracingItemNotWellFormatted(t *testing.T) {
	raw, err := os.ReadFile("./testdata/invalid_logs_payload.json")
	require.NoError(t, err)
	sanitizedData := removeInvalidTracingItem(raw)
	sanitizedRaw, sanitizedErr := os.ReadFile("./testdata/invalid_logs_payload_sanitized.json")
	require.NoError(t, sanitizedErr)
	assert.Equal(t, string(sanitizedRaw), string(sanitizedData))
}

func TestParseLogsAPIPayloadWellFormated(t *testing.T) {
	raw, err := os.ReadFile("./testdata/valid_logs_payload.json")
	require.NoError(t, err)
	messages, err := parseLogsAPIPayload(raw)
	assert.Nil(t, err)
	assert.NotNil(t, messages)
	assert.NotNil(t, messages[0].objectRecord.requestID)
}

func TestParseLogsAPIPayloadNotWellFormated(t *testing.T) {
	raw, err := os.ReadFile("./testdata/invalid_logs_payload.json")
	require.NoError(t, err)
	messages, err := parseLogsAPIPayload(raw)
	assert.Nil(t, err)
	assert.NotNil(t, messages[0].objectRecord.requestID)
}

func TestParseLogsAPIPayloadNotWellFormatedButNotRecoverable(t *testing.T) {
	raw, err := os.ReadFile("./testdata/invalid_logs_payload_unrecoverable.json")
	require.NoError(t, err)
	_, err = parseLogsAPIPayload(raw)
	assert.NotNil(t, err)
}

func TestProcessMessageValid(t *testing.T) {
	demux := aggregator.InitTestAgentDemultiplexerWithFlushInterval(time.Hour)
	defer demux.Stop(false)

	message := LambdaLogAPIMessage{
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
	tags := Tags{
		Tags: metricTags,
	}
	lc := NewLambdaLogCollector(make(chan<- *config.ChannelMessage), demux, &tags, true, computeEnhancedMetrics, mockExecutionContext, func() {}, make(chan<- float64))

	lc.processMessage(&message)

	received, timed := demux.WaitForNumberOfSamples(7, 0, 100*time.Millisecond)
	assert.Len(t, received, 7)
	assert.Len(t, timed, 0)
	demux.Reset()

	lc.enhancedMetricsEnabled = false
	lc.processMessage(&message)

	received, timed = demux.WaitForSamples(100 * time.Millisecond)
	assert.Len(t, received, 0, "we should NOT have received metrics")
	assert.Len(t, timed, 0)
}

func TestProcessMessageStartValid(t *testing.T) {
	demux := aggregator.InitTestAgentDemultiplexerWithFlushInterval(time.Hour)
	defer demux.Stop(false)

	message := &LambdaLogAPIMessage{
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
	tags := Tags{
		Tags: metricTags,
	}
	lc := NewLambdaLogCollector(make(chan<- *config.ChannelMessage), demux, &tags, true, computeEnhancedMetrics, mockExecutionContext, mockRuntimeDone, make(chan<- float64))
	lc.lastRequestID = lastRequestID
	lc.processMessage(message)
	assert.Equal(t, runtimeDoneCallbackWasCalled, false)
}

func TestProcessMessagePlatformRuntimeDoneValid(t *testing.T) {
	demux := aggregator.InitTestAgentDemultiplexerWithFlushInterval(time.Hour)
	messageTime := time.Now()
	defer demux.Stop(false)
	message := LambdaLogAPIMessage{
		logType: logTypePlatformRuntimeDone,
		time:    messageTime,
		objectRecord: platformObjectRecord{
			requestID: "8286a188-ba32-4475-8077-530cd35c09a9",
		},
	}
	arn := "arn:aws:lambda:us-east-1:123456789012:function:test-function"
	lastRequestID := "8286a188-ba32-4475-8077-530cd35c09a9"
	metricTags := []string{"functionname:test-function"}
	tags := Tags{
		Tags: metricTags,
	}
	computeEnhancedMetrics := true

	mockExecutionContext := &executioncontext.ExecutionContext{}
	mockExecutionContext.SetFromInvocation(arn, lastRequestID)

	runtimeDoneCallbackWasCalled := false
	mockRuntimeDone := func() {
		runtimeDoneCallbackWasCalled = true
	}
	lc := NewLambdaLogCollector(make(chan<- *config.ChannelMessage), demux, &tags, true, computeEnhancedMetrics, mockExecutionContext, mockRuntimeDone, make(chan<- float64))
	lc.lastRequestID = lastRequestID
	lc.processMessage(&message)
	ecs := mockExecutionContext.GetCurrentState()
	assert.Equal(t, runtimeDoneCallbackWasCalled, true)
	assert.WithinDuration(t, messageTime, ecs.EndTime, time.Millisecond)
}

func TestProcessMessagePlatformRuntimeDonePreviousInvocation(t *testing.T) {
	demux := aggregator.InitTestAgentDemultiplexerWithFlushInterval(time.Hour)
	defer demux.Stop(false)

	previousRequestID := "9397b299-cb43-5586-9188-641de46d10b0"
	currentRequestID := "8286a188-ba32-4475-8077-530cd35c09a9"
	message := &LambdaLogAPIMessage{
		logType: logTypePlatformRuntimeDone,
		time:    time.Now(),
		objectRecord: platformObjectRecord{
			requestID: previousRequestID,
		},
	}
	arn := "arn:aws:lambda:us-east-1:123456789012:function:test-function"
	lastRequestID := currentRequestID
	metricTags := []string{"functionname:test-function"}
	tags := Tags{
		Tags: metricTags,
	}

	computeEnhancedMetrics := true
	mockExecutionContext := &executioncontext.ExecutionContext{}
	mockExecutionContext.SetFromInvocation(arn, lastRequestID)

	runtimeDoneCallbackWasCalled := false
	mockRuntimeDone := func() {
		runtimeDoneCallbackWasCalled = true
	}
	lc := NewLambdaLogCollector(make(chan<- *config.ChannelMessage), demux, &tags, true, computeEnhancedMetrics, mockExecutionContext, mockRuntimeDone, make(chan<- float64))

	lc.processMessage(message)
	// Runtime done callback should NOT be called if the log message was for a previous invocation
	assert.Equal(t, runtimeDoneCallbackWasCalled, false)
}

func TestProcessMessageShouldNotProcessArnNotSet(t *testing.T) {
	demux := aggregator.InitTestAgentDemultiplexerWithFlushInterval(time.Hour)
	defer demux.Stop(false)
	message := &LambdaLogAPIMessage{
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
	tags := Tags{
		Tags: metricTags,
	}

	mockExecutionContext := &executioncontext.ExecutionContext{}

	computeEnhancedMetrics := true
	lc := NewLambdaLogCollector(make(chan<- *config.ChannelMessage), demux, &tags, true, computeEnhancedMetrics, mockExecutionContext, func() {}, make(chan<- float64))

	go lc.processMessage(message)

	received, timed := demux.WaitForSamples(100 * time.Millisecond)
	assert.Len(t, received, 0, "We should NOT have received metrics")
	assert.Len(t, timed, 0)
}

func TestProcessMessageShouldNotProcessLogsDropped(t *testing.T) {
	demux := aggregator.InitTestAgentDemultiplexerWithFlushInterval(time.Hour)
	defer demux.Stop(false)
	message := &LambdaLogAPIMessage{
		logType:      logTypePlatformLogsDropped,
		time:         time.Now(),
		stringRecord: "bla bla bla",
	}

	arn := "arn:aws:lambda:us-east-1:123456789012:function:test-function"
	lastRequestID := "8286a188-ba32-4475-8077-530cd35c09a9"
	metricTags := []string{"functionname:test-function"}
	tags := Tags{
		Tags: metricTags,
	}
	computeEnhancedMetrics := true

	mockExecutionContext := &executioncontext.ExecutionContext{}
	mockExecutionContext.SetFromInvocation(arn, lastRequestID)
	lc := NewLambdaLogCollector(make(chan<- *config.ChannelMessage), demux, &tags, true, computeEnhancedMetrics, mockExecutionContext, func() {}, make(chan<- float64))

	go lc.processMessage(message)

	received, timed := demux.WaitForSamples(100 * time.Millisecond)
	assert.Len(t, received, 0, "We should NOT have received metrics")
	assert.Len(t, timed, 0)
}

func TestProcessMessageShouldProcessLogTypeFunction(t *testing.T) {
	demux := aggregator.InitTestAgentDemultiplexerWithFlushInterval(time.Hour)
	defer demux.Stop(false)
	message := &LambdaLogAPIMessage{
		logType:      logTypeFunction,
		time:         time.Now(),
		stringRecord: "fatal error: runtime: out of memory",
	}

	arn := "arn:aws:lambda:us-east-1:123456789012:function:test-function"
	lastRequestID := "8286a188-ba32-4475-8077-530cd35c09a9"
	metricTags := []string{"functionname:test-function"}
	tags := Tags{
		Tags: metricTags,
	}
	computeEnhancedMetrics := true

	mockExecutionContext := &executioncontext.ExecutionContext{}
	mockExecutionContext.SetFromInvocation(arn, lastRequestID)

	lc := NewLambdaLogCollector(make(chan<- *config.ChannelMessage), demux, &tags, true, computeEnhancedMetrics, mockExecutionContext, func() {}, make(chan<- float64))

	go lc.processMessage(message)

	received, timed := demux.WaitForNumberOfSamples(2, 0, 100*time.Millisecond)
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
		arn:           "my-arn",
		lastRequestID: "myRequestID",
		logsEnabled:   true,
		out:           logChannel,
		extraTags: &Tags{
			Tags: []string{"tag0:value0,tag1:value1"},
		},
		executionContext: mockExecutionContext,
	}

	logMessages := []LambdaLogAPIMessage{
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

	go logCollection.processLogMessages(logMessages)

	select {
	case received := <-logChannel:
		assert.NotNil(t, received)
		assert.Equal(t, "my-arn", received.Lambda.ARN)
		assert.Equal(t, "myRequestID", received.Lambda.RequestID)
	case <-time.After(100 * time.Millisecond):
		assert.Fail(t, "We should have received logs")
	}
}

// Verify sorting result, and request ID overwrite
func TestProcessLogMessageLogsEnabledForMixedUnorderedMessages(t *testing.T) {

	logChannel := make(chan *config.ChannelMessage)

	mockExecutionContext := &executioncontext.ExecutionContext{}
	mockExecutionContext.SetFromInvocation("my-arn", "myRequestID")

	logCollection := &LambdaLogsCollector{
		logsEnabled:   true,
		arn:           "my-arn",
		lastRequestID: "myRequestID",
		out:           logChannel,
		extraTags: &Tags{
			Tags: []string{"tag0:value0,tag1:value1"},
		},
		executionContext: mockExecutionContext,
	}

	logMessages := []LambdaLogAPIMessage{
		{
			stringRecord: "hi, log 3", time: time.UnixMilli(12345678), objectRecord: platformObjectRecord{requestID: "3th ID"},
		},
		{
			stringRecord: "hi, log 1", time: time.UnixMilli(123456), logType: logTypePlatformStart, objectRecord: platformObjectRecord{requestID: "2nd ID"},
		},
		{
			stringRecord: "hi, log 2", time: time.UnixMilli(1234567),
		},
		{
			stringRecord: "hi, log 0", time: time.UnixMilli(12345), objectRecord: platformObjectRecord{requestID: "1st ID"},
		},
	}
	go logCollection.processLogMessages(logMessages)

	expectedRequestIDs := [4]string{"1st ID", "2nd ID", "2nd ID", "3th ID"}

	for i := 0; i < 4; i++ {
		select {
		case received := <-logChannel:
			assert.NotNil(t, received)
			assert.Equal(t, "my-arn", received.Lambda.ARN)
			assert.Equal(t, expectedRequestIDs[i], received.Lambda.RequestID)
		case <-time.After(100 * time.Millisecond):
			assert.Fail(t, "We should have received logs")
		}
	}
}

func TestProcessLogMessageNoStringRecordPlatformLog(t *testing.T) {

	logChannel := make(chan *config.ChannelMessage)

	mockExecutionContext := &executioncontext.ExecutionContext{}
	mockExecutionContext.SetFromInvocation("my-arn", "myRequestID")
	logCollection := &LambdaLogsCollector{
		logsEnabled: true,
		out:         logChannel,
		extraTags: &Tags{
			Tags: []string{"tag0:value0,tag1:value1"},
		},
		executionContext: mockExecutionContext,
	}

	logMessages := []LambdaLogAPIMessage{
		{
			logType: logTypePlatformRuntimeDone,
		},
	}
	go logCollection.processLogMessages(logMessages)

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
		logsEnabled:   true,
		out:           logChannel,
		arn:           "my-arn",
		lastRequestID: "myRequestID",
		extraTags: &Tags{
			Tags: []string{"tag0:value0,tag1:value1"},
		},
		executionContext: mockExecutionContext,
	}

	logMessages := []LambdaLogAPIMessage{
		{
			stringRecord: "hi, log 2",
		},
	}
	go logCollection.processLogMessages(logMessages)

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
		logsEnabled: false,
		out:         logChannel,
		extraTags: &Tags{
			Tags: []string{"tag0:value0,tag1:value1"},
		},
		executionContext: mockExecutionContext,
	}

	logMessages := []LambdaLogAPIMessage{
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
	go logCollection.processLogMessages(logMessages)

	select {
	case <-logChannel:
		assert.Fail(t, "We should not have received logs")
	case <-time.After(100 * time.Millisecond):
		// nothing to do here
	}
}

func TestServeHTTPInvalidPayload(t *testing.T) {
	logChannel := make(chan []LambdaLogAPIMessage)

	mockExecutionContext := &executioncontext.ExecutionContext{}
	mockExecutionContext.SetFromInvocation("my-arn", "myRequestID")
	logAPI := &LambdaLogsAPIServer{
		out: logChannel,
	}

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	res := httptest.NewRecorder()

	logAPI.ServeHTTP(res, req)
	assert.Equal(t, 400, res.Code)
}

func TestServeHTTPSuccess(t *testing.T) {
	logChannel := make(chan []LambdaLogAPIMessage, 1000)

	mockExecutionContext := &executioncontext.ExecutionContext{}
	mockExecutionContext.SetFromInvocation("my-arn", "myRequestID")

	logAPI := &LambdaLogsAPIServer{
		out: logChannel,
	}

	raw, err := os.ReadFile("./testdata/extension_log.json")
	if err != nil {
		assert.Fail(t, "should be able to read the log file")
	}
	if err != nil {
		assert.Fail(t, "should be able to marshal")
	}
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(raw))
	res := httptest.NewRecorder()

	logAPI.ServeHTTP(res, req)
	assert.Equal(t, 200, res.Code)
}

func TestUnmarshalJSONInvalid(t *testing.T) {
	logMessage := &LambdaLogAPIMessage{}
	err := logMessage.UnmarshalJSON([]byte("invalid"))
	assert.NotNil(t, err)
}

func TestUnmarshalJSONMalformed(t *testing.T) {
	logMessage := &LambdaLogAPIMessage{}
	raw, errReadFile := os.ReadFile("./testdata/invalid_log_no_type.json")
	if errReadFile != nil {
		assert.Fail(t, "should be able to read the file")
	}
	err := logMessage.UnmarshalJSON(raw)
	assert.NotNil(t, err)
}

func TestUnmarshalJSONLogTypePlatformLogsSubscription(t *testing.T) {
	// with the telemetry api, these events should not exist
	logMessage := &LambdaLogAPIMessage{}
	raw, errReadFile := os.ReadFile("./testdata/platform_log.json")
	if errReadFile != nil {
		assert.Fail(t, "should be able to read the file")
	}
	err := logMessage.UnmarshalJSON(raw)
	assert.Nil(t, err)
	assert.Equal(t, "", logMessage.logType)
}

func TestUnmarshalJSONLogTypePlatformFault(t *testing.T) {
	// platform.fault events are not processed by the extension
	logMessage := &LambdaLogAPIMessage{}
	raw, errReadFile := os.ReadFile("./testdata/platform_fault.json")
	if errReadFile != nil {
		assert.Fail(t, "should be able to read the file")
	}
	err := logMessage.UnmarshalJSON(raw)
	assert.Nil(t, err)
	assert.Equal(t, "", logMessage.logType)
}

func TestUnmarshalJSONLogTypePlatformTelemetrySubscription(t *testing.T) {
	// with the telemetry api, these events should not exist
	logMessage := &LambdaLogAPIMessage{}
	raw, errReadFile := os.ReadFile("./testdata/platform_telemetry.json")
	if errReadFile != nil {
		assert.Fail(t, "should be able to read the file")
	}
	err := logMessage.UnmarshalJSON(raw)
	assert.Nil(t, err)
	assert.Equal(t, "", logMessage.logType)
}

func TestUnmarshalJSONLogTypePlatformExtension(t *testing.T) {
	// platform.extension events are not processed by the extension
	logMessage := &LambdaLogAPIMessage{}
	raw, errReadFile := os.ReadFile("./testdata/platform_extension.json")
	if errReadFile != nil {
		assert.Fail(t, "should be able to read the file")
	}
	err := logMessage.UnmarshalJSON(raw)
	assert.Nil(t, err)
	assert.Equal(t, "", logMessage.logType)
}

func TestUnmarshalJSONLogTypePlatformStart(t *testing.T) {
	logMessage := &LambdaLogAPIMessage{}
	raw, errReadFile := os.ReadFile("./testdata/platform_start.json")
	if errReadFile != nil {
		assert.Fail(t, "should be able to read the file")
	}
	err := logMessage.UnmarshalJSON(raw)
	assert.Nil(t, err)
	assert.Equal(t, "platform.start", logMessage.logType)
	assert.Equal(t, "START RequestId: 13dee504-0d50-4c86-8d82-efd20693afc9 Version: 10", logMessage.stringRecord)
}

func TestUnmarshalJSONLogTypePlatformEnd(t *testing.T) {
	// with the telemetry api, these events should not exist
	logMessage := &LambdaLogAPIMessage{}
	raw, errReadFile := os.ReadFile("./testdata/platform_end.json")
	if errReadFile != nil {
		assert.Fail(t, "should be able to read the file")
	}
	err := logMessage.UnmarshalJSON(raw)
	assert.Nil(t, err)
	assert.Equal(t, "", logMessage.logType)
	assert.Equal(t, "", logMessage.stringRecord)
}

func TestUnmarshalJSONLogTypeIncorrectReportNotFatalMetrics(t *testing.T) {
	logMessage := &LambdaLogAPIMessage{}
	raw, errReadFile := os.ReadFile("./testdata/platform_incorrect_report.json")
	if errReadFile != nil {
		assert.Fail(t, "should be able to read the file")
	}
	err := logMessage.UnmarshalJSON(raw)
	assert.Nil(t, err)
}

func TestUnmarshalJSONLogTypeIncorrectReportNotFatalReport(t *testing.T) {
	logMessage := &LambdaLogAPIMessage{}
	raw, errReadFile := os.ReadFile("./testdata/platform_incorrect_report_record.json")
	if errReadFile != nil {
		assert.Fail(t, "should be able to read the file")
	}
	err := logMessage.UnmarshalJSON(raw)
	assert.Nil(t, err)
}

func TestUnmarshalPlatformRuntimeDoneLog(t *testing.T) {
	raw, err := os.ReadFile("./testdata/platform_runtime_done_log_valid.json")
	require.NoError(t, err)
	var message LambdaLogAPIMessage
	err = json.Unmarshal(raw, &message)
	require.NoError(t, err)

	expectedTime := time.Date(2021, 05, 19, 18, 11, 22, 478000000, time.UTC)

	expectedLogMessage := LambdaLogAPIMessage{
		logType:      logTypePlatformRuntimeDone,
		time:         expectedTime,
		stringRecord: "END RequestId: 13dee504-0d50-4c86-8d82-efd20693afc9",
		objectRecord: platformObjectRecord{
			requestID: "13dee504-0d50-4c86-8d82-efd20693afc9",
		},
	}
	assert.Equal(t, expectedLogMessage, message)
}

func TestUnmarshalPlatformRuntimeDoneLogWithTelemetry(t *testing.T) {
	raw, err := os.ReadFile("./testdata/platform_runtime_done_log_valid_with_telemetry.json")
	require.NoError(t, err)
	var message LambdaLogAPIMessage
	err = json.Unmarshal(raw, &message)
	require.NoError(t, err)

	expectedTime := time.Date(2021, 05, 19, 18, 11, 22, 478000000, time.UTC)

	expectedLogMessage := LambdaLogAPIMessage{
		logType:      logTypePlatformRuntimeDone,
		time:         expectedTime,
		stringRecord: "END RequestId: 13dee504-0d50-4c86-8d82-efd20693afc9",
		objectRecord: platformObjectRecord{
			requestID: "13dee504-0d50-4c86-8d82-efd20693afc9",
			runtimeDoneItem: runtimeDoneItem{
				responseDuration: 0.1,
				responseLatency:  6.0,
				producedBytes:    53,
			},
		},
	}
	assert.Equal(t, expectedLogMessage, message)
}

func TestUnmarshalPlatformRuntimeDoneLogNotFatal(t *testing.T) {
	logMessage := &LambdaLogAPIMessage{}
	raw, errReadFile := os.ReadFile("./testdata/platform_incorrect_runtime_done_log.json")
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
	mockExecutionContext.UpdateStartTime(startTime)
	computeEnhancedMetrics := true
	tags := Tags{
		Tags: []string{},
	}

	doneMessage := &LambdaLogAPIMessage{
		time:    endTime,
		logType: logTypePlatformRuntimeDone,
		objectRecord: platformObjectRecord{
			requestID: requestID,
		},
	}
	reportMessage := &LambdaLogAPIMessage{
		time:    reportLogTime,
		logType: logTypePlatformReport,
		objectRecord: platformObjectRecord{
			requestID: requestID,
			reportLogItem: reportLogMetrics{
				durationMs: durationMs,
			},
		},
	}
	lc := NewLambdaLogCollector(make(chan<- *config.ChannelMessage), demux, &tags, true, computeEnhancedMetrics, mockExecutionContext, func() {}, make(chan<- float64))
	lc.invocationStartTime = startTime

	lc.processMessage(doneMessage)
	lc.processMessage(reportMessage)

	generatedMetrics, timedMetrics := demux.WaitForNumberOfSamples(10, 0, 100*time.Millisecond)
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
	assert.Equal(t, generatedMetrics[7], metrics.MetricSample{
		Name:       "aws.lambda.enhanced.duration",
		Value:      durationMs / 1000, // in seconds
		Mtype:      metrics.DistributionType,
		Tags:       []string{"cold_start:true"},
		SampleRate: 1,
		Timestamp:  postRuntimeMetricTimestamp,
	})
	assert.Equal(t, generatedMetrics[9], metrics.MetricSample{
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
