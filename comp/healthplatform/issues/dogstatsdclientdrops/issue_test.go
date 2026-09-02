// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package dogstatsdclientdrops

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/agent-payload/v5/healthplatform"
)

func TestBuildUDSIssue(t *testing.T) {
	tests := []struct {
		name                 string
		context              UDSDetectionContext
		expectedTitle        string
		expectedTransport    string
		expectedDescription  string
		expectedBytes        string
		expectedBreakdown    string
		expectedComplete     bool
		expectedUnclassified float64
	}{
		{
			name: "UDS drop",
			context: UDSDetectionContext{
				ClientLibrary:               ClientLibraryGo,
				AgentHostname:               "test-host",
				DroppedRatio:                0.02,
				Threshold:                   0.01,
				BytesSent:                   980,
				BytesDropped:                20,
				BytesDroppedQueue:           12,
				BytesDroppedWriter:          8,
				DropReasonBreakdownComplete: true,
			},
			expectedTitle:       "Sustained DogStatsD Go UDS payload drops detected by Agent test-host",
			expectedTransport:   transportFamilyUDS,
			expectedDescription: "2.0000% payload-byte drop rate",
			expectedBytes:       "dropped=20.00",
			expectedBreakdown:   "queue=12.00, writer=8.00",
			expectedComplete:    true,
		},
		{
			name: "fractional near-threshold values remain visible",
			context: UDSDetectionContext{
				ClientLibrary:            ClientLibraryGo,
				AgentHostname:            "test-host",
				DroppedRatio:             0.010001,
				Threshold:                0.01,
				BytesSent:                49.5,
				BytesDropped:             0.5,
				BytesDroppedUnclassified: 0.5,
			},
			expectedTitle:        "Sustained DogStatsD Go UDS payload drops detected by Agent test-host",
			expectedTransport:    transportFamilyUDS,
			expectedDescription:  "1.0001% payload-byte drop rate",
			expectedBytes:        "dropped=0.50",
			expectedBreakdown:    "partial breakdown: queue=0.00, writer=0.00, unclassified=0.50",
			expectedUnclassified: 0.5,
		},
		{
			name:                "missing hostname uses safe default",
			context:             UDSDetectionContext{ClientLibrary: ClientLibraryGo},
			expectedTitle:       "Sustained DogStatsD Go UDS payload drops detected by Agent unknown host",
			expectedTransport:   transportFamilyUDS,
			expectedDescription: "0.0000% payload-byte drop rate",
			expectedBytes:       "dropped=0.00",
			expectedBreakdown:   "partial breakdown: queue=0.00, writer=0.00, unclassified=0.00",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			issue, err := BuildUDSIssue(test.context)
			require.NoError(t, err)
			library := NormalizeClientLibrary(string(test.context.ClientLibrary))
			assert.Empty(t, issue.Id)
			assert.Equal(t, UDSIssueName(library), issue.IssueName)
			assert.Equal(t, UDSIssueType(library), issue.IssueType)
			assert.Equal(t, healthplatform.IssueSeverity_ISSUE_SEVERITY_LOW, issue.Severity)
			assert.Equal(t, test.expectedTitle, issue.Title)
			assert.Contains(t, issue.Description, test.expectedDescription)
			assert.Contains(t, issue.Description, "above the ")
			assert.Contains(t, issue.Description, test.expectedBytes)
			assert.Contains(t, issue.Description, test.expectedBreakdown)
			assert.Contains(t, issue.Tags, test.expectedTransport)
			assert.Contains(t, issue.Tags, "dogstatsd")
			assert.Equal(t, category, issue.Category)
			assert.Equal(t, location, issue.Location)
			assert.Equal(t, source, issue.Source)
			require.NotNil(t, issue.Extra)
			fields := issue.Extra.GetFields()
			assert.Contains(t, issue.Tags, "host:"+fields[contextKeyHostname].GetStringValue())
			assert.Equal(t, test.expectedTransport, fields[contextKeyTransportFamily].GetStringValue())
			assert.Equal(t, string(library), fields[contextKeyClientLibrary].GetStringValue())
			assert.True(t, fields[contextKeyDetectionEvidenceAvailable].GetBoolValue())
			assert.Contains(t, fields, contextKeyHostname)
			assert.Contains(t, fields, contextKeyDroppedRatio)
			assert.Contains(t, fields, contextKeyThreshold)
			assert.Contains(t, fields, contextKeyBytesSent)
			assert.Contains(t, fields, contextKeyBytesDropped)
			assert.Contains(t, fields, contextKeyBytesDroppedQueue)
			assert.Contains(t, fields, contextKeyBytesDroppedWriter)
			assert.Equal(t, test.expectedUnclassified, fields[contextKeyBytesDroppedUnclassified].GetNumberValue())
			assert.Equal(t, test.expectedComplete, fields[contextKeyDropReasonBreakdownComplete].GetBoolValue())
			require.NotEmpty(t, issue.Remediation.Steps)
			assert.EqualValues(t, 1, issue.Remediation.Steps[0].Order)
			assert.Contains(t, issue.Remediation.Steps[0].Text, "datadog.dogstatsd.client.bytes_dropped")
			assert.Contains(t, issue.Remediation.Steps[len(issue.Remediation.Steps)-1].Text, highThroughputDocs)
		})
	}
}

func TestBuildUDSIssueRejectsUnsupportedLibrary(t *testing.T) {
	issue, err := BuildUDSIssue(UDSDetectionContext{ClientLibrary: "ruby", AgentHostname: "test-host"})
	require.Error(t, err)
	assert.Nil(t, issue)
}

func TestBuildRestoredUDSIssue(t *testing.T) {
	issue, err := BuildRestoredUDSIssue(ClientLibraryPython, "test-host")
	require.NoError(t, err)
	assert.Empty(t, issue.Id)
	assert.Equal(t, UDSIssueName(ClientLibraryPython), issue.IssueName)
	assert.Equal(t, UDSIssueType(ClientLibraryPython), issue.IssueType)
	assert.Equal(t, healthplatform.IssueSeverity_ISSUE_SEVERITY_MEDIUM, issue.Severity)
	assert.Equal(t, "Sustained DogStatsD Python UDS payload drops detected by Agent test-host", issue.Title)
	assert.Contains(t, issue.Description, "previously detected")
	assert.Contains(t, issue.Description, "awaiting current client telemetry")
	require.NotNil(t, issue.Extra)
	fields := issue.Extra.GetFields()
	assert.Equal(t, "test-host", fields[contextKeyHostname].GetStringValue())
	assert.Equal(t, "py", fields[contextKeyClientLibrary].GetStringValue())
	assert.Equal(t, transportFamilyUDS, fields[contextKeyTransportFamily].GetStringValue())
	assert.False(t, fields[contextKeyDetectionEvidenceAvailable].GetBoolValue())
	assert.NotContains(t, fields, contextKeyDroppedRatio)
	assert.NotContains(t, fields, contextKeyBytesSent)
	assert.NotContains(t, fields, contextKeyBytesDropped)
	require.NotEmpty(t, issue.Remediation.Steps)
}

func TestSeverityForDroppedRatio(t *testing.T) {
	for _, test := range []struct {
		ratio    float64
		severity healthplatform.IssueSeverity
	}{
		{ratio: 0.0499, severity: healthplatform.IssueSeverity_ISSUE_SEVERITY_LOW},
		{ratio: 0.05, severity: healthplatform.IssueSeverity_ISSUE_SEVERITY_MEDIUM},
		{ratio: 0.2499, severity: healthplatform.IssueSeverity_ISSUE_SEVERITY_MEDIUM},
		{ratio: 0.25, severity: healthplatform.IssueSeverity_ISSUE_SEVERITY_HIGH},
	} {
		require.Equal(t, test.severity, SeverityForDroppedRatio(test.ratio))
	}
}

func TestLibrarySpecificUDSRemediation(t *testing.T) {
	for _, test := range []struct {
		library     ClientLibrary
		contains    []string
		notContains []string
	}{
		{library: ClientLibraryGo, contains: []string{"client:go", "WithoutClientSideAggregation", "WithSenderQueueSize", "WithErrorHandler", "code-lang=go"}},
		{library: ClientLibraryPython, contains: []string{"client:py", "statsd_disable_aggregation=False", "sender_queue_timeout", "socket_connect_timeout", "code-lang=python"}},
		{library: ClientLibraryJava, contains: []string{"client:java", "errorHandler", "enableAggregation(true)", "connectionTimeout", "code-lang=java"}, notContains: []string{bytesDroppedQueueMetric, bytesDroppedWriterMetric}},
	} {
		t.Run(string(test.library), func(t *testing.T) {
			issue, err := BuildUDSIssue(UDSDetectionContext{ClientLibrary: test.library, AgentHostname: "test-host"})
			require.NoError(t, err)
			var remediation strings.Builder
			for _, step := range issue.Remediation.Steps {
				remediation.WriteString(step.Text)
			}
			for _, expected := range test.contains {
				assert.Contains(t, remediation.String(), expected)
			}
			for _, unexpected := range test.notContains {
				assert.NotContains(t, remediation.String(), unexpected)
			}
		})
	}
}

func TestStableUDSIssueIdentity(t *testing.T) {
	assert.Equal(t, "DogStatsD Go UDS Client Payload Drops", UDSIssueName(ClientLibraryGo))
	assert.Equal(t, "dogstatsd_go_uds_client_payload_drops", UDSIssueType(ClientLibraryGo))
	assert.Equal(t, "dogstatsd-go-uds-client-payload-drops:uuid:test-uuid", UDSIssueIDForHost(ClientLibraryGo, "test-uuid", "test-host"))
	assert.Equal(t, "dogstatsd-go-uds-client-payload-drops:hostname:test-host", UDSIssueIDForHost(ClientLibraryGo, "", "test-host"))
	assert.Equal(t, UDSIssueIDForHost(ClientLibraryGo, "test-uuid", "old-host"), UDSIssueIDForHost(ClientLibraryGo, "test-uuid", "new-host"))
	assert.NotEqual(t, UDSIssueIDForHost(ClientLibraryGo, "test-uuid", "test-host"), UDSIssueIDForHost(ClientLibraryPython, "test-uuid", "test-host"))
	assert.NotEqual(t, UDSIssueIDForHost(ClientLibraryGo, "first-uuid", "test-host"), UDSIssueIDForHost(ClientLibraryGo, "second-uuid", "test-host"))
	assert.NotEqual(t, UDSIssueIDForHost(ClientLibraryGo, "", "first-host"), UDSIssueIDForHost(ClientLibraryGo, "", "second-host"))
}
