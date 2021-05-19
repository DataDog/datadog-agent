// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aws

import (
	"encoding/json"
	"io/ioutil"
	"testing"
	"time"

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

	assert.True(t, ShouldProcessLog(nonEmptyARN, nonEmptyRequestID, validLog))
	assert.False(t, ShouldProcessLog(emptyARN, emptyRequestID, validLog))
	assert.False(t, ShouldProcessLog(nonEmptyARN, emptyRequestID, validLog))
	assert.False(t, ShouldProcessLog(emptyARN, nonEmptyRequestID, validLog))

	assert.False(t, ShouldProcessLog(nonEmptyARN, nonEmptyRequestID, invalidLog0))
	assert.False(t, ShouldProcessLog(emptyARN, emptyRequestID, invalidLog0))
	assert.False(t, ShouldProcessLog(nonEmptyARN, emptyRequestID, invalidLog0))
	assert.False(t, ShouldProcessLog(emptyARN, nonEmptyRequestID, invalidLog0))

	assert.False(t, ShouldProcessLog(nonEmptyARN, nonEmptyRequestID, invalidLog1))
	assert.False(t, ShouldProcessLog(emptyARN, emptyRequestID, invalidLog1))
	assert.False(t, ShouldProcessLog(nonEmptyARN, emptyRequestID, invalidLog1))
	assert.False(t, ShouldProcessLog(emptyARN, nonEmptyRequestID, invalidLog1))
}

func TestCreateStringRecordForReportLogWithInitDuration(t *testing.T) {
	var sampleLogMessage = LogMessage{
		ObjectRecord: PlatformObjectRecord{
			RequestID: "cf84ebaf-606a-4b0f-b99b-3685bfe973d7",
			Metrics: ReportLogMetrics{
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
		ObjectRecord: PlatformObjectRecord{
			RequestID: "cf84ebaf-606a-4b0f-b99b-3685bfe973d7",
			Metrics: ReportLogMetrics{
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
	messages, err := ParseLogsAPIPayload(raw)
	assert.Nil(t, err)
	assert.NotNil(t, messages)
	assert.NotNil(t, messages[0].ObjectRecord.RequestID)
}

func TestParseLogsAPIPayloadNotWellFormated(t *testing.T) {
	raw, err := ioutil.ReadFile("./testdata/invalid_logs_payload.json")
	require.NoError(t, err)
	messages, err := ParseLogsAPIPayload(raw)
	assert.Nil(t, err)
	assert.NotNil(t, messages[0].ObjectRecord.RequestID)
}

func TestParseLogsAPIPayloadNotWellFormatedButNotRecoverable(t *testing.T) {
	raw, err := ioutil.ReadFile("./testdata/invalid_logs_payload_unrecoverable.json")
	require.NoError(t, err)
	_, err = ParseLogsAPIPayload(raw)
	assert.NotNil(t, err)
}
