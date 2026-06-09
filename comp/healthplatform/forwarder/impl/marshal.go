// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package forwarderimpl

import (
	"encoding/json"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/types/known/structpb"
)

// issueJSON is a JSON-serializable mirror of healthplatform.Issue that emits
// severity as a lowercase string ("low", "medium", "high") instead of the
// integer enum value that encoding/json produces for IssueSeverity (int32).
//
// The agenthealth-remediation EVP worker maps this JSON directly onto the
// recommendations.Issue proto, whose severity field is typed string.
// Sending an integer caused the field to be silently dropped, so all
// severity-filtered tile queries returned zero results.
//
// PersistedIssue is intentionally omitted: it is internal lifecycle state and
// has no corresponding field in the downstream recommendations proto.
type issueJSON struct {
	ID          string                      `json:"id,omitempty"`
	IssueName   string                      `json:"issue_name,omitempty"`
	Title       string                      `json:"title,omitempty"`
	Description string                      `json:"description,omitempty"`
	Category    string                      `json:"category,omitempty"`
	Location    string                      `json:"location,omitempty"`
	Severity    string                      `json:"severity,omitempty"`
	DetectedAt  string                      `json:"detected_at,omitempty"`
	Source      string                      `json:"source,omitempty"`
	Extra       *structpb.Struct            `json:"extra,omitempty"`
	Remediation *healthplatform.Remediation `json:"remediation,omitempty"`
	Tags        []string                    `json:"tags,omitempty"`
}

type hostInfoJSON struct {
	Hostname     string   `json:"hostname,omitempty"`
	AgentVersion *string  `json:"agent_version,omitempty"`
	ParIds       []string `json:"par_ids,omitempty"`
}

type healthReportJSON struct {
	SchemaVersion string                `json:"schema_version,omitempty"`
	EventType     string                `json:"event_type,omitempty"`
	EmittedAt     string                `json:"emitted_at,omitempty"`
	Host          *hostInfoJSON         `json:"host,omitempty"`
	Issues        map[string]*issueJSON `json:"issues,omitempty"`
}

// severityString converts an IssueSeverity enum value to the lowercase string
// expected by the recommendations pipeline ("low", "medium", "high").
func severityString(s healthplatform.IssueSeverity) string {
	switch s {
	case healthplatform.IssueSeverity_ISSUE_SEVERITY_LOW:
		return "low"
	case healthplatform.IssueSeverity_ISSUE_SEVERITY_MEDIUM:
		return "medium"
	case healthplatform.IssueSeverity_ISSUE_SEVERITY_HIGH:
		return "high"
	default:
		return ""
	}
}

// marshalHealthReport serializes a HealthReport to JSON, converting the
// IssueSeverity integer enum to a lowercase string for each issue so that the
// downstream agenthealth-remediation service can forward it without loss.
func marshalHealthReport(report *healthplatform.HealthReport) ([]byte, error) {
	w := &healthReportJSON{
		SchemaVersion: report.GetSchemaVersion(),
		EventType:     report.GetEventType(),
		EmittedAt:     report.GetEmittedAt(),
	}
	if h := report.GetHost(); h != nil {
		w.Host = &hostInfoJSON{
			Hostname:     h.GetHostname(),
			AgentVersion: h.AgentVersion,
			ParIds:       h.GetParIds(),
		}
	}
	if issues := report.GetIssues(); len(issues) > 0 {
		w.Issues = make(map[string]*issueJSON, len(issues))
		for k, issue := range issues {
			w.Issues[k] = &issueJSON{
				ID:          issue.GetId(),
				IssueName:   issue.GetIssueName(),
				Title:       issue.GetTitle(),
				Description: issue.GetDescription(),
				Category:    issue.GetCategory(),
				Location:    issue.GetLocation(),
				Severity:    severityString(issue.GetSeverity()),
				DetectedAt:  issue.GetDetectedAt(),
				Source:      issue.GetSource(),
				Extra:       issue.GetExtra(),
				Remediation: issue.GetRemediation(),
				Tags:        issue.GetTags(),
			}
		}
	}
	return json.Marshal(w)
}
