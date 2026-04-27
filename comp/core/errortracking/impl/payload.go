// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package errortrackingimpl

// Envelope shapes for the apmtelemetry-style payload posted to
// /api/v2/apmtelemetry. Field names and JSON tags are pinned by Worker 4's
// ARCH_NOTES_coat_intake.md §2 — the dd-go agenttelemetry processor dispatches
// on RequestType and reads payload fields by JSON key, so changes here must
// be coordinated with the receiver.

// Payload is the top-level envelope expected by the apmtelemetry intake.
type Payload struct {
	APIVersion  string        `json:"api_version"`
	RequestType string        `json:"request_type"`
	EventTime   int64         `json:"event_time"`
	DebugFlag   bool          `json:"debug,omitempty"`
	Host        HostPayload   `json:"host"`
	Payload     ErrorTracking `json:"payload"`
}

// HostPayload is the host-identification block of the envelope. The intake
// currently treats it as opaque and derives the real host from the API key,
// so we only emit a hostname value.
type HostPayload struct {
	Hostname string `json:"hostname"`
}

// ErrorTracking is the application-specific payload for COAT error records.
// AgentVersion / Hostname / Service describe the originating agent; Records
// holds the per-log entries Worker 1's pipeline batched up.
type ErrorTracking struct {
	AgentVersion string        `json:"agent_version"`
	Hostname     string        `json:"hostname"`
	Service      string        `json:"service"`
	OrgID        int64         `json:"org_id,omitempty"`
	Records      []ErrorRecord `json:"records"`
}

// ErrorRecord is one slog.Record flattened for the wire. Attrs is a flat
// string→string map for v1; structured attrs are deferred until we have a
// receiver-side schema for them.
type ErrorRecord struct {
	Time    string            `json:"time"`
	Level   string            `json:"level"`
	Message string            `json:"message"`
	Caller  string            `json:"caller,omitempty"`
	Attrs   map[string]string `json:"attrs,omitempty"`
}
