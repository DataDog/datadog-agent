// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logs

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	logConfig "github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/scheduler"
	"github.com/DataDog/datadog-agent/pkg/logs/service"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	serverlessMetrics "github.com/DataDog/datadog-agent/pkg/serverless/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnmarshalExtensionLog(t *testing.T) {
	raw, err := ioutil.ReadFile("./testdata/extension_log.json")
	require.NoError(t, err)
	var messages []LogMessage
	err = json.Unmarshal(raw, &messages)
	require.NoError(t, err)

	expectedTime, _ := time.Parse(logMessageTimeLayout, "2020-08-20T12:31:32.123Z")
	expectedLogMessage := LogMessage{
		Type:         LogTypeExtension,
		Time:         expectedTime,
		StringRecord: "sample extension log",
	}
	assert.Equal(t, expectedLogMessage, messages[0])
}

func TestShouldProcessLog(t *testing.T) {

	validLog := LogMessage{
		Type: LogTypePlatformReport,
	}

	invalidLog0 := LogMessage{
		Type: LogTypePlatformLogsSubscription,
	}

	invalidLog1 := LogMessage{
		Type: LogTypePlatformExtension,
	}

	nonEmptyARN := "arn:aws:lambda:us-east-1:123456789012:function:my-function"
	emptyARN := ""

	nonEmptyRequestID := "8286a188-ba32-4475-8077-530cd35c09a9"
	emptyRequestID := ""

	assert.True(t, shouldProcessLog(&ExecutionContext{ARN: nonEmptyARN, LastRequestID: nonEmptyRequestID}, validLog))
	assert.False(t, shouldProcessLog(&ExecutionContext{ARN: emptyARN, LastRequestID: emptyRequestID}, validLog))
	assert.False(t, shouldProcessLog(&ExecutionContext{ARN: nonEmptyARN, LastRequestID: emptyRequestID}, validLog))
	assert.False(t, shouldProcessLog(&ExecutionContext{ARN: emptyARN, LastRequestID: nonEmptyRequestID}, validLog))

	assert.False(t, shouldProcessLog(&ExecutionContext{ARN: nonEmptyARN, LastRequestID: nonEmptyRequestID}, invalidLog0))
	assert.False(t, shouldProcessLog(&ExecutionContext{ARN: emptyARN, LastRequestID: emptyRequestID}, invalidLog0))
	assert.False(t, shouldProcessLog(&ExecutionContext{ARN: nonEmptyARN, LastRequestID: emptyRequestID}, invalidLog0))
	assert.False(t, shouldProcessLog(&ExecutionContext{ARN: emptyARN, LastRequestID: nonEmptyRequestID}, invalidLog0))

	assert.False(t, shouldProcessLog(&ExecutionContext{ARN: nonEmptyARN, LastRequestID: nonEmptyRequestID}, invalidLog1))
	assert.False(t, shouldProcessLog(&ExecutionContext{ARN: emptyARN, LastRequestID: emptyRequestID}, invalidLog1))
	assert.False(t, shouldProcessLog(&ExecutionContext{ARN: nonEmptyARN, LastRequestID: emptyRequestID}, invalidLog1))
	assert.False(t, shouldProcessLog(&ExecutionContext{ARN: emptyARN, LastRequestID: nonEmptyRequestID}, invalidLog1))
}

func TestCreateStringRecordForReportLogWithInitDuration(t *testing.T) {
	var sampleLogMessage = LogMessage{
		ObjectRecord: serverlessMetrics.PlatformObjectRecord{
			RequestID: "cf84ebaf-606a-4b0f-b99b-3685bfe973d7",
			Metrics: serverlessMetrics.ReportLogMetrics{
				DurationMs:       100.00,
				BilledDurationMs: 100,
				MemorySizeMB:     128,
				MaxMemoryUsedMB:  128,
				InitDurationMs:   50.00,
			},
		},
	}
	output := createStringRecordForReportLog(&sampleLogMessage)
	expectedOutput := "REPORT RequestId: cf84ebaf-606a-4b0f-b99b-3685bfe973d7\tDuration: 100.00 ms\tBilled Duration: 100 ms\tMemory Size: 128 MB\tMax Memory Used: 128 MB\tInit Duration: 50.00 ms"
	assert.Equal(t, expectedOutput, output)
}

func TestCreateStringRecordForReportLogWithoutInitDuration(t *testing.T) {
	var sampleLogMessage = LogMessage{
		ObjectRecord: serverlessMetrics.PlatformObjectRecord{
			RequestID: "cf84ebaf-606a-4b0f-b99b-3685bfe973d7",
			Metrics: serverlessMetrics.ReportLogMetrics{
				DurationMs:       100.00,
				BilledDurationMs: 100,
				MemorySizeMB:     128,
				MaxMemoryUsedMB:  128,
				InitDurationMs:   0.00,
			},
		},
	}
	output := createStringRecordForReportLog(&sampleLogMessage)
	expectedOutput := "REPORT RequestId: cf84ebaf-606a-4b0f-b99b-3685bfe973d7\tDuration: 100.00 ms\tBilled Duration: 100 ms\tMemory Size: 128 MB\tMax Memory Used: 128 MB"
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
	assert.NotNil(t, messages[0].ObjectRecord.RequestID)
}

func TestParseLogsAPIPayloadNotWellFormated(t *testing.T) {
	raw, err := ioutil.ReadFile("./testdata/invalid_logs_payload.json")
	require.NoError(t, err)
	messages, err := parseLogsAPIPayload(raw)
	assert.Nil(t, err)
	assert.NotNil(t, messages[0].ObjectRecord.RequestID)
}

func TestParseLogsAPIPayloadNotWellFormatedButNotRecoverable(t *testing.T) {
	raw, err := ioutil.ReadFile("./testdata/invalid_logs_payload_unrecoverable.json")
	require.NoError(t, err)
	_, err = parseLogsAPIPayload(raw)
	assert.NotNil(t, err)
}

func TestGetLambdaSourceNilScheduler(t *testing.T) {
	assert.Nil(t, GetLambdaSource())
}

func TestGetLambdaSourceNilSource(t *testing.T) {
	logSources := config.NewLogSources()
	services := service.NewServices()
	scheduler.CreateScheduler(logSources, services)
	assert.Nil(t, GetLambdaSource())
}

func TestGetLambdaSourceValidSource(t *testing.T) {
	logSources := config.NewLogSources()
	chanSource := config.NewLogSource("TestLog", &config.LogsConfig{
		Type:    config.StringChannelType,
		Source:  "lambda",
		Tags:    nil,
		Channel: nil,
	})
	logSources.AddSource(chanSource)
	services := service.NewServices()
	scheduler.CreateScheduler(logSources, services)
	assert.NotNil(t, GetLambdaSource())
}

func TestProcessMessageValid(t *testing.T) {
	message := LogMessage{
		Type: LogTypePlatformReport,
		Time: time.Now(),
		ObjectRecord: serverlessMetrics.PlatformObjectRecord{
			Metrics: serverlessMetrics.ReportLogMetrics{
				DurationMs:       1000.0,
				BilledDurationMs: 800.0,
				MemorySizeMB:     1024.0,
				MaxMemoryUsedMB:  256.0,
				InitDurationMs:   100.0,
			},
		},
	}
	arn := "arn:aws:lambda:us-east-1:123456789012:function:test-function"
	lastRequestID := "8286a188-ba32-4475-8077-530cd35c09a9"
	metricTags := []string{"functionname:test-function"}

	metricsChan := make(chan []metrics.MetricSample, 1)
	computeEnhancedMetrics := true
	go processMessage(message, &ExecutionContext{ARN: arn, LastRequestID: lastRequestID}, computeEnhancedMetrics, metricTags, metricsChan)

	select {
	case received := <-metricsChan:
		assert.Equal(t, len(received), 6)
	case <-time.After(100 * time.Millisecond):
		assert.Fail(t, "We should have received metrics")
	}

	metricsChan = make(chan []metrics.MetricSample, 1)
	computeEnhancedMetrics = false
	go processMessage(message, &ExecutionContext{ARN: arn, LastRequestID: lastRequestID}, computeEnhancedMetrics, metricTags, metricsChan)

	select {
	case <-metricsChan:
		assert.Fail(t, "We should NOT have received metrics")
	case <-time.After(100 * time.Millisecond):
		//nothing to do here
	}
}

func TestProcessMessageStartValid(t *testing.T) {
	message := LogMessage{
		Type: LogTypePlatformStart,
		Time: time.Now(),
		ObjectRecord: serverlessMetrics.PlatformObjectRecord{
			RequestID: "8286a188-ba32-4475-8077-530cd35c09a9",
		},
	}
	arn := "arn:aws:lambda:us-east-1:123456789012:function:test-function"
	lastRequestID := "8286a188-ba32-4475-8077-530cd35c09a9"
	metricTags := []string{"functionname:test-function"}

	metricsChan := make(chan []metrics.MetricSample, 1)
	executionContext := &ExecutionContext{ARN: arn, LastRequestID: lastRequestID}
	computeEnhancedMetrics := true
	processMessage(message, executionContext, computeEnhancedMetrics, metricTags, metricsChan)
	assert.Equal(t, lastRequestID, executionContext.LastLogRequestID)
}

func TestProcessMessageShouldNotProcess(t *testing.T) {
	message := LogMessage{
		Type: LogTypePlatformReport,
		Time: time.Now(),
		ObjectRecord: serverlessMetrics.PlatformObjectRecord{
			Metrics: serverlessMetrics.ReportLogMetrics{
				DurationMs:       1000.0,
				BilledDurationMs: 800.0,
				MemorySizeMB:     1024.0,
				MaxMemoryUsedMB:  256.0,
				InitDurationMs:   100.0,
			},
		},
	}

	metricTags := []string{"functionname:test-function"}

	metricsChan := make(chan []metrics.MetricSample, 1)
	computeEnhancedMetrics := true
	go processMessage(message, &ExecutionContext{ARN: "", LastRequestID: ""}, computeEnhancedMetrics, metricTags, metricsChan)

	select {
	case <-metricsChan:
		assert.Fail(t, "We should NOT have received metrics")
	case <-time.After(100 * time.Millisecond):
		//nothing to do here
	}
}

func TestProcessMessageShouldNotProcessLogsDropped(t *testing.T) {
	message := LogMessage{
		Type:         LogTypePlatformLogsDropped,
		Time:         time.Now(),
		StringRecord: "bla bla bla",
	}

	arn := "arn:aws:lambda:us-east-1:123456789012:function:test-function"
	lastRequestID := "8286a188-ba32-4475-8077-530cd35c09a9"
	metricTags := []string{"functionname:test-function"}

	metricsChan := make(chan []metrics.MetricSample, 1)
	computeEnhancedMetrics := true
	go processMessage(message, &ExecutionContext{ARN: arn, LastRequestID: lastRequestID}, computeEnhancedMetrics, metricTags, metricsChan)

	select {
	case <-metricsChan:
		assert.Fail(t, "We should NOT have received metrics")
	case <-time.After(100 * time.Millisecond):
		//nothing to do here
	}
}

func TestProcessMessageShouldProcessLogTypeFunction(t *testing.T) {
	message := LogMessage{
		Type:         LogTypeFunction,
		Time:         time.Now(),
		StringRecord: "fatal error: runtime: out of memory",
	}

	arn := "arn:aws:lambda:us-east-1:123456789012:function:test-function"
	lastRequestID := "8286a188-ba32-4475-8077-530cd35c09a9"
	metricTags := []string{"functionname:test-function"}

	metricsChan := make(chan []metrics.MetricSample, 1)
	computeEnhancedMetrics := true
	go processMessage(message, &ExecutionContext{ARN: arn, LastRequestID: lastRequestID}, computeEnhancedMetrics, metricTags, metricsChan)

	select {
	case received := <-metricsChan:
		assert.Equal(t, len(received), 1)
		assert.Equal(t, "aws.lambda.enhanced.out_of_memory", received[0].Name)
	case <-time.After(100 * time.Millisecond):
		assert.Fail(t, "We should have received metrics")
	}
}

func TestProcessLogMessageLogsEnabled(t *testing.T) {

	logChannel := make(chan *logConfig.ChannelMessage)

	logCollection := &CollectionRouteInfo{
		ExecutionContext: &ExecutionContext{
			ARN:           "myARN",
			LastRequestID: "myRequestID",
		},
		LogsEnabled: true,
		LogChannel:  logChannel,
		ExtraTags: &Tags{
			Tags: []string{"tag0:value0,tag1:value1"},
		},
	}

	logMessages := []LogMessage{
		{
			StringRecord: "hi, log 0",
		},
		{
			StringRecord: "hi, log 1",
		},
		{
			StringRecord: "hi, log 2",
		},
	}
	go processLogMessages(logCollection, logMessages)

	select {
	case received := <-logChannel:
		assert.NotNil(t, received)
		assert.Equal(t, "myARN", received.Lambda.ARN)
		assert.Equal(t, "myRequestID", received.Lambda.RequestID)
	case <-time.After(100 * time.Millisecond):
		assert.Fail(t, "We should have received logs")
	}
}

func TestProcessLogMessageLogsNotEnabled(t *testing.T) {

	logChannel := make(chan *logConfig.ChannelMessage)

	logCollection := &CollectionRouteInfo{
		ExecutionContext: &ExecutionContext{
			ARN:           "myARN",
			LastRequestID: "myRequestID",
		},
		LogsEnabled: false,
		LogChannel:  logChannel,
		ExtraTags: &Tags{
			Tags: []string{"tag0:value0,tag1:value1"},
		},
	}

	logMessages := []LogMessage{
		{
			StringRecord: "hi, log 0",
		},
		{
			StringRecord: "hi, log 1",
		},
		{
			StringRecord: "hi, log 2",
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
	logChannel := make(chan *logConfig.ChannelMessage)

	logCollection := &CollectionRouteInfo{
		ExecutionContext: &ExecutionContext{
			ARN:           "myARN",
			LastRequestID: "myRequestID",
		},
		LogsEnabled: false,
		LogChannel:  logChannel,
		ExtraTags: &Tags{
			Tags: []string{"tag0:value0,tag1:value1"},
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	res := httptest.NewRecorder()

	logCollection.ServeHTTP(res, req)
	assert.Equal(t, 400, res.Code)
}

func TestServeHTTPSuccess(t *testing.T) {
	logChannel := make(chan *logConfig.ChannelMessage)

	logCollection := &CollectionRouteInfo{
		ExecutionContext: &ExecutionContext{
			ARN:           "myARN",
			LastRequestID: "myRequestID",
		},
		LogsEnabled: false,
		LogChannel:  logChannel,
		ExtraTags: &Tags{
			Tags: []string{"tag0:value0,tag1:value1"},
		},
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
	logMessage := &LogMessage{}
	err := logMessage.UnmarshalJSON([]byte("invalid"))
	assert.NotNil(t, err)
}

func TestUnmarshalJSONMalformed(t *testing.T) {
	logMessage := &LogMessage{}
	raw, errReadFile := ioutil.ReadFile("./testdata/invalid_log_no_type.json")
	if errReadFile != nil {
		assert.Fail(t, "should be able to read the file")
	}
	err := logMessage.UnmarshalJSON(raw)
	assert.NotNil(t, err)
}

func TestUnmarshalJSONLogTypePlatformLogsSubscription(t *testing.T) {
	logMessage := &LogMessage{}
	raw, errReadFile := ioutil.ReadFile("./testdata/platform_log.json")
	if errReadFile != nil {
		assert.Fail(t, "should be able to read the file")
	}
	err := logMessage.UnmarshalJSON(raw)
	assert.Nil(t, err)
	assert.Equal(t, "platform.logsSubscription", logMessage.Type)
}

func TestUnmarshalJSONLogTypePlatformStart(t *testing.T) {
	logMessage := &LogMessage{}
	raw, errReadFile := ioutil.ReadFile("./testdata/platform_start.json")
	if errReadFile != nil {
		assert.Fail(t, "should be able to read the file")
	}
	err := logMessage.UnmarshalJSON(raw)
	assert.Nil(t, err)
	assert.Equal(t, "platform.start", logMessage.Type)
	assert.Equal(t, "START RequestId: 13dee504-0d50-4c86-8d82-efd20693afc9 Version: 10", logMessage.StringRecord)
}

func TestUnmarshalJSONLogTypePlatformEnd(t *testing.T) {
	logMessage := &LogMessage{}
	raw, errReadFile := ioutil.ReadFile("./testdata/platform_end.json")
	if errReadFile != nil {
		assert.Fail(t, "should be able to read the file")
	}
	err := logMessage.UnmarshalJSON(raw)
	assert.Nil(t, err)
	assert.Equal(t, "platform.end", logMessage.Type)
	assert.Equal(t, "END RequestId: 13dee504-0d50-4c86-8d82-efd20693afc9", logMessage.StringRecord)
}

func TestUnmarshalJSONLogTypeIncorrectReportNotFatalMetrics(t *testing.T) {
	logMessage := &LogMessage{}
	raw, errReadFile := ioutil.ReadFile("./testdata/platform_incorrect_report.json")
	if errReadFile != nil {
		assert.Fail(t, "should be able to read the file")
	}
	err := logMessage.UnmarshalJSON(raw)
	assert.Nil(t, err)
}

func TestUnmarshalJSONLogTypeIncorrectReportNotFatalReport(t *testing.T) {
	logMessage := &LogMessage{}
	raw, errReadFile := ioutil.ReadFile("./testdata/platform_incorrect_report_record.json")
	if errReadFile != nil {
		assert.Fail(t, "should be able to read the file")
	}
	err := logMessage.UnmarshalJSON(raw)
	assert.Nil(t, err)
}
