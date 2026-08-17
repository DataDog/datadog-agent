// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package dogstatsdclientdrops

import (
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
				Hostname:                    "test-host",
				DroppedRatio:                0.02,
				Threshold:                   0.01,
				BytesSent:                   980,
				BytesDropped:                20,
				BytesDroppedQueue:           12,
				BytesDroppedWriter:          8,
				DropReasonBreakdownComplete: true,
			},
			expectedTitle:       "Sustained DogStatsD UDS payload drops detected on test-host",
			expectedTransport:   transportFamilyUDS,
			expectedDescription: "2.0000% payload-byte drop rate",
			expectedBytes:       "dropped=20.00",
			expectedBreakdown:   "queue=12.00, writer=8.00",
			expectedComplete:    true,
		},
		{
			name: "fractional near-threshold values remain visible",
			context: UDSDetectionContext{
				Hostname:                 "test-host",
				DroppedRatio:             0.010001,
				Threshold:                0.01,
				BytesSent:                49.5,
				BytesDropped:             0.5,
				BytesDroppedUnclassified: 0.5,
			},
			expectedTitle:        "Sustained DogStatsD UDS payload drops detected on test-host",
			expectedTransport:    transportFamilyUDS,
			expectedDescription:  "1.0001% payload-byte drop rate",
			expectedBytes:        "dropped=0.50",
			expectedBreakdown:    "partial breakdown: queue=0.00, writer=0.00, unclassified=0.50",
			expectedUnclassified: 0.5,
		},
		{
			name:                "missing context uses safe defaults",
			expectedTitle:       "Sustained DogStatsD UDS payload drops detected on unknown host",
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
			assert.Empty(t, issue.Id)
			assert.Equal(t, UDSIssueName, issue.IssueName)
			assert.Equal(t, UDSIssueType, issue.IssueType)
			assert.Equal(t, healthplatform.IssueSeverity_ISSUE_SEVERITY_MEDIUM, issue.Severity)
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
			assert.Equal(t, test.expectedTransport, fields[contextKeyTransportFamily].GetStringValue())
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
			require.Len(t, issue.Remediation.Steps, 3)
			assert.Equal(t, "Identify the source of DogStatsD UDS payload drops, then reduce queue or writer pressure.", issue.Remediation.Summary)
			assert.EqualValues(t, 1, issue.Remediation.Steps[0].Order)
			assert.EqualValues(t, 2, issue.Remediation.Steps[1].Order)
			assert.EqualValues(t, 3, issue.Remediation.Steps[2].Order)
			assert.Contains(t, issue.Remediation.Steps[0].Text, "datadog.dogstatsd.client.bytes_dropped_queue")
			assert.Contains(t, issue.Remediation.Steps[0].Text, "datadog.dogstatsd.client.bytes_dropped_writer")
			assert.Contains(t, issue.Remediation.Steps[1].Text, "reduce both queue and writer pressure")
			assert.Contains(t, issue.Remediation.Steps[1].Text, "sender queue capacity")
			assert.Contains(t, issue.Remediation.Steps[1].Text, "write and connection timeouts")
			assert.Equal(t, "Redeploy the affected application.", issue.Remediation.Steps[2].Text)
		})
	}
}

func TestBuildRestoredUDSIssue(t *testing.T) {
	issue, err := BuildRestoredUDSIssue("test-host")
	require.NoError(t, err)
	assert.Empty(t, issue.Id)
	assert.Equal(t, UDSIssueName, issue.IssueName)
	assert.Equal(t, UDSIssueType, issue.IssueType)
	assert.Equal(t, "Sustained DogStatsD UDS payload drops detected on test-host", issue.Title)
	assert.Contains(t, issue.Description, "previously detected")
	assert.Contains(t, issue.Description, "awaiting current client telemetry")
	require.NotNil(t, issue.Extra)
	fields := issue.Extra.GetFields()
	assert.Equal(t, "test-host", fields[contextKeyHostname].GetStringValue())
	assert.Equal(t, transportFamilyUDS, fields[contextKeyTransportFamily].GetStringValue())
	assert.False(t, fields[contextKeyDetectionEvidenceAvailable].GetBoolValue())
	assert.NotContains(t, fields, contextKeyDroppedRatio)
	assert.NotContains(t, fields, contextKeyBytesSent)
	assert.NotContains(t, fields, contextKeyBytesDropped)
	require.Len(t, issue.Remediation.Steps, 3)
}

func TestStableUDSIssueIdentity(t *testing.T) {
	assert.Equal(t, "DogStatsD UDS Client Payload Drops", UDSIssueName)
	assert.Equal(t, "dogstatsd_uds_client_payload_drops", UDSIssueType)
	assert.Equal(t, "dogstatsd-uds-client-payload-drops", UDSIssueID)
	assert.Equal(t, "dogstatsd-uds-client-payload-drops:97864685c4a5b06a", UDSIssueIDForHostname("test-host"))
	assert.NotEqual(t, UDSIssueIDForHostname("test-host"), UDSIssueIDForHostname("other-host"))
}
