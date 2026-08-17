// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package dogstatsdclientdrops provides Agent Health issues for DogStatsD
// client-reported payload drops.
package dogstatsdclientdrops

import (
	"fmt"
	"hash/fnv"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	// UDSIssueName is the stable, human-readable UDS issue name.
	UDSIssueName = "DogStatsD UDS Client Payload Drops"
	// UDSIssueType is the stable backend type key for the UDS issue.
	UDSIssueType = "dogstatsd_uds_client_payload_drops"
	// UDSIssueID is the stable base for Agent-scoped UDS issue IDs.
	UDSIssueID = "dogstatsd-uds-client-payload-drops"

	udsIssueName = UDSIssueName
	udsIssueType = UDSIssueType

	category = "dogstatsd"
	location = "dogstatsd"
	severity = healthplatform.IssueSeverity_ISSUE_SEVERITY_MEDIUM
	source   = "dogstatsd"

	transportFamilyUDS = "uds"

	contextKeyHostname                    = "hostname"
	contextKeyTransportFamily             = "transport_family"
	contextKeyDroppedRatio                = "dropped_ratio"
	contextKeyThreshold                   = "threshold"
	contextKeyBytesSent                   = "bytes_sent"
	contextKeyBytesDropped                = "bytes_dropped"
	contextKeyBytesDroppedQueue           = "bytes_dropped_queue"
	contextKeyBytesDroppedWriter          = "bytes_dropped_writer"
	contextKeyBytesDroppedUnclassified    = "bytes_dropped_unclassified"
	contextKeyDropReasonBreakdownComplete = "drop_reason_breakdown_complete"
	contextKeyDetectionEvidenceAvailable  = "detection_evidence_available"
)

// UDSDetectionContext contains the values from the confirmation window that
// triggered the UDS issue.
type UDSDetectionContext struct {
	Hostname                    string
	DroppedRatio                float64
	Threshold                   float64
	BytesSent                   float64
	BytesDropped                float64
	BytesDroppedQueue           float64
	BytesDroppedWriter          float64
	BytesDroppedUnclassified    float64
	DropReasonBreakdownComplete bool
}

// UDSIssueIDForHostname returns a deterministic UDS issue ID scoped to one Agent/node.
func UDSIssueIDForHostname(hostname string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(hostname))
	return fmt.Sprintf("%s:%016x", UDSIssueID, h.Sum64())
}

// BuildUDSIssue creates a complete Agent Health issue from a confirmed UDS violation.
func BuildUDSIssue(context UDSDetectionContext) (*healthplatform.Issue, error) {
	return buildUDSIssue(context, false)
}

// BuildRestoredUDSIssue recreates an active UDS issue after an Agent restart
// when its original detection evidence is no longer available in memory.
func BuildRestoredUDSIssue(hostname string) (*healthplatform.Issue, error) {
	return buildUDSIssue(UDSDetectionContext{Hostname: hostname}, true)
}

func buildUDSIssue(context UDSDetectionContext, restored bool) (*healthplatform.Issue, error) {
	hostname := context.Hostname
	if hostname == "" {
		hostname = "unknown host"
	}
	extraValues := map[string]any{
		contextKeyHostname:                   hostname,
		contextKeyTransportFamily:            transportFamilyUDS,
		contextKeyDetectionEvidenceAvailable: !restored,
		"impact":                             "DogStatsD clients on affected hosts reported sustained UDS payload-byte loss. Metrics, events, service checks, and telemetry emitted by those clients may be missing from Datadog.",
	}

	title := "Sustained DogStatsD UDS payload drops detected on " + hostname
	description := "The Agent previously detected sustained DogStatsD UDS client payload drops and is awaiting current client telemetry after restart."
	if !restored {
		extraValues[contextKeyDroppedRatio] = context.DroppedRatio
		extraValues[contextKeyThreshold] = context.Threshold
		extraValues[contextKeyBytesSent] = context.BytesSent
		extraValues[contextKeyBytesDropped] = context.BytesDropped
		extraValues[contextKeyBytesDroppedQueue] = context.BytesDroppedQueue
		extraValues[contextKeyBytesDroppedWriter] = context.BytesDroppedWriter
		extraValues[contextKeyBytesDroppedUnclassified] = context.BytesDroppedUnclassified
		extraValues[contextKeyDropReasonBreakdownComplete] = context.DropReasonBreakdownComplete

		breakdown := fmt.Sprintf("queue=%.2f, writer=%.2f", context.BytesDroppedQueue, context.BytesDroppedWriter)
		if !context.DropReasonBreakdownComplete {
			breakdown = fmt.Sprintf("partial breakdown: queue=%.2f, writer=%.2f, unclassified=%.2f", context.BytesDroppedQueue, context.BytesDroppedWriter, context.BytesDroppedUnclassified)
		}
		description = fmt.Sprintf(
			"DogStatsD clients using UDS reported a %.4f%% payload-byte drop rate during the observation period, above the %.4f%% threshold (dropped=%.2f, sent=%.2f; %s).",
			context.DroppedRatio*100,
			context.Threshold*100,
			context.BytesDropped,
			context.BytesSent,
			breakdown,
		)
	}

	extra, err := structpb.NewStruct(extraValues)
	if err != nil {
		return nil, fmt.Errorf("failed to create DogStatsD client drop issue context: %w", err)
	}

	return &healthplatform.Issue{
		IssueName:   udsIssueName,
		IssueType:   udsIssueType,
		Title:       title,
		Description: description,
		Category:    category,
		Location:    location,
		Severity:    severity,
		Source:      source,
		Extra:       extra,
		Tags:        []string{"dogstatsd", "client", "payload-drops", transportFamilyUDS},
		Remediation: buildUDSRemediation(),
	}, nil
}

func buildUDSRemediation() *healthplatform.Remediation {
	return &healthplatform.Remediation{
		Summary: "Identify the source of DogStatsD UDS payload drops, then reduce queue or writer pressure.",
		Steps: []*healthplatform.RemediationStep{
			{Order: 1, Text: "In Metrics Explorer, review datadog.dogstatsd.client.bytes_dropped_queue and datadog.dogstatsd.client.bytes_dropped_writer for the affected host."},
			{Order: 2, Text: "If client-side aggregation is not enabled, enable it to reduce both queue and writer pressure. If queue drops continue, increase the sender queue capacity; if writer drops continue, increase the UDS write and connection timeouts."},
			{Order: 3, Text: "Redeploy the affected application."},
		},
	}
}
