// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package dogstatsdclientdrops provides Agent Health issues for DogStatsD
// client-reported payload drops.
package dogstatsdclientdrops

import (
	"fmt"
	"strings"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	category = "dogstatsd"
	location = "dogstatsd"
	source   = "dogstatsd"

	mediumSeverityDroppedRatio = 0.05
	highSeverityDroppedRatio   = 0.25

	transportFamilyUDS = "uds"

	contextKeyHostname                    = "hostname"
	contextKeyClientLibrary               = "client_library"
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
// triggered a library-specific UDS issue.
type UDSDetectionContext struct {
	ClientLibrary               ClientLibrary
	AgentHostname               string
	DroppedRatio                float64
	Threshold                   float64
	BytesSent                   float64
	BytesDropped                float64
	BytesDroppedQueue           float64
	BytesDroppedWriter          float64
	BytesDroppedUnclassified    float64
	DropReasonBreakdownComplete bool
}

// NormalizeClientLibrary normalizes a telemetry client-library tag.
func NormalizeClientLibrary(clientLibrary string) ClientLibrary {
	return ClientLibrary(strings.ToLower(clientLibrary))
}

// IsSupportedClientLibrary reports whether drop detection is implemented for a library.
func IsSupportedClientLibrary(clientLibrary ClientLibrary) bool {
	_, found := issueDefinitions[clientLibrary]
	return found
}

// ClientLibraries returns the client-library buckets maintained by the detector.
func ClientLibraries() []ClientLibrary {
	libraries := make([]ClientLibrary, 0, len(issueDefinitions))
	for library := range issueDefinitions {
		libraries = append(libraries, library)
	}
	return libraries
}

// UDSIssueName returns the stable issue name for a client library.
func UDSIssueName(clientLibrary ClientLibrary) string {
	return definitionFor(clientLibrary).issueName
}

// UDSIssueType returns the stable issue type for a client library.
func UDSIssueType(clientLibrary ClientLibrary) string {
	return definitionFor(clientLibrary).issueType
}

// UDSIssueIDForHost returns a deterministic library-specific UDS issue ID for one Agent/node.
func UDSIssueIDForHost(clientLibrary ClientLibrary, hostUUID, agentHostname string) string {
	base := definitionFor(clientLibrary).issueID
	if hostUUID != "" {
		return fmt.Sprintf("%s:uuid:%s", base, hostUUID)
	}
	return fmt.Sprintf("%s:hostname:%s", base, agentHostname)
}

// SeverityForDroppedRatio maps the dropped-byte ratio to an Agent Health severity.
func SeverityForDroppedRatio(ratio float64) healthplatform.IssueSeverity {
	if ratio >= highSeverityDroppedRatio {
		return healthplatform.IssueSeverity_ISSUE_SEVERITY_HIGH
	}
	if ratio >= mediumSeverityDroppedRatio {
		return healthplatform.IssueSeverity_ISSUE_SEVERITY_MEDIUM
	}
	return healthplatform.IssueSeverity_ISSUE_SEVERITY_LOW
}

// BuildUDSIssue creates an Agent Health issue from a confirmed UDS violation.
func BuildUDSIssue(context UDSDetectionContext) (*healthplatform.Issue, error) {
	context.ClientLibrary = NormalizeClientLibrary(string(context.ClientLibrary))
	if !IsSupportedClientLibrary(context.ClientLibrary) {
		return nil, fmt.Errorf("unsupported DogStatsD client library %q", context.ClientLibrary)
	}
	return buildUDSIssue(context, false)
}

// BuildRestoredUDSIssue recreates an active UDS issue after an Agent restart
// when its original detection evidence is no longer available in memory.
func BuildRestoredUDSIssue(clientLibrary ClientLibrary, agentHostname string) (*healthplatform.Issue, error) {
	clientLibrary = NormalizeClientLibrary(string(clientLibrary))
	if !IsSupportedClientLibrary(clientLibrary) {
		return nil, fmt.Errorf("unsupported DogStatsD client library %q", clientLibrary)
	}
	return buildUDSIssue(UDSDetectionContext{ClientLibrary: clientLibrary, AgentHostname: agentHostname}, true)
}

func definitionFor(clientLibrary ClientLibrary) issueDefinition {
	return issueDefinitions[clientLibrary]
}

func buildUDSIssue(context UDSDetectionContext, restored bool) (*healthplatform.Issue, error) {
	definition := definitionFor(context.ClientLibrary)
	agentHostname := context.AgentHostname
	if agentHostname == "" {
		agentHostname = "unknown host"
	}
	extraValues := map[string]any{
		contextKeyHostname:                   agentHostname,
		contextKeyClientLibrary:              string(definition.library),
		contextKeyTransportFamily:            transportFamilyUDS,
		contextKeyDetectionEvidenceAvailable: !restored,
		"impact":                             fmt.Sprintf("DogStatsD %s clients reporting through the affected Agent reported sustained UDS payload-byte loss. Metrics, events, service checks, and telemetry emitted by those clients may be missing from Datadog.", definition.displayName),
	}

	title := fmt.Sprintf("Sustained DogStatsD %s UDS payload drops detected by Agent %s", definition.displayName, agentHostname)
	description := fmt.Sprintf("The Agent previously detected sustained UDS payload drops from DogStatsD %s clients reporting through it and is awaiting current client telemetry after restart.", definition.displayName)
	if !restored {
		extraValues[contextKeyDroppedRatio] = context.DroppedRatio
		extraValues[contextKeyThreshold] = context.Threshold
		extraValues[contextKeyBytesSent] = context.BytesSent
		extraValues[contextKeyBytesDropped] = context.BytesDropped
		extraValues[contextKeyBytesDroppedQueue] = context.BytesDroppedQueue
		extraValues[contextKeyBytesDroppedWriter] = context.BytesDroppedWriter
		extraValues[contextKeyBytesDroppedUnclassified] = context.BytesDroppedUnclassified
		extraValues[contextKeyDropReasonBreakdownComplete] = context.DropReasonBreakdownComplete

		breakdown := ""
		if len(definition.dropReasonMetrics) > 0 {
			breakdown = fmt.Sprintf("; queue=%.2f, writer=%.2f", context.BytesDroppedQueue, context.BytesDroppedWriter)
			if !context.DropReasonBreakdownComplete {
				breakdown = fmt.Sprintf("; partial breakdown: queue=%.2f, writer=%.2f, unclassified=%.2f", context.BytesDroppedQueue, context.BytesDroppedWriter, context.BytesDroppedUnclassified)
			}
		}
		description = fmt.Sprintf(
			"DogStatsD %s clients reporting through this Agent using UDS reported a %.4f%% payload-byte drop rate during the observation period, above the %.4f%% threshold (dropped=%.2f, sent=%.2f%s).",
			definition.displayName,
			context.DroppedRatio*100,
			context.Threshold*100,
			context.BytesDropped,
			context.BytesSent,
			breakdown,
		)
	}

	issueSeverity := SeverityForDroppedRatio(context.DroppedRatio)
	if restored {
		issueSeverity = healthplatform.IssueSeverity_ISSUE_SEVERITY_MEDIUM
	}
	issue := &healthplatform.Issue{
		IssueName:   definition.issueName,
		IssueType:   definition.issueType,
		Title:       title,
		Description: description,
		Category:    category,
		Location:    location,
		Severity:    issueSeverity,
		Source:      source,
		Tags:        []string{"dogstatsd", "client", "client:" + string(definition.library), "host:" + agentHostname, "payload-drops", transportFamilyUDS},
		Remediation: buildUDSRemediation(definition, agentHostname),
	}

	extra, err := structpb.NewStruct(extraValues)
	if err != nil {
		return issue, fmt.Errorf("failed to create DogStatsD client drop issue diagnostic details: %w", err)
	}
	issue.Extra = extra
	return issue, nil
}

func buildUDSRemediation(definition issueDefinition, agentHostname string) *healthplatform.Remediation {
	metricFilter := fmt.Sprintf("host:%s and client:%s", agentHostname, definition.library)
	docsURL := highThroughputDocs + "?code-lang=" + strings.ToLower(definition.displayName)
	metricNames := bytesDroppedMetric
	if len(definition.dropReasonMetrics) > 0 {
		metricNames = strings.Join(definition.dropReasonMetrics, " and ")
	}
	steps := []*healthplatform.RemediationStep{
		{Order: 1, Text: fmt.Sprintf("In Metrics Explorer, review %s filtered by %s. The host identifies the receiving Agent.", metricNames, metricFilter)},
	}
	for _, guidance := range definition.remediationGuidance {
		steps = append(steps, &healthplatform.RemediationStep{Order: int32(len(steps) + 1), Text: guidance})
	}

	steps = append(steps, &healthplatform.RemediationStep{
		Order: int32(len(steps) + 1),
		Text:  fmt.Sprintf("See %s for additional %s guidance, then redeploy the affected application.", docsURL, definition.displayName),
	})

	return &healthplatform.Remediation{
		Summary: fmt.Sprintf("Reduce DogStatsD %s UDS payload drops reported through Agent %s.", definition.displayName, agentHostname),
		Steps:   steps,
	}
}
