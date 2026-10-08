// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
)

// sqlNonParsableResource is the resource the trace-agent sets on a SQL span whose
// query it failed to obfuscate. The intake never copies it into db.statement.
const sqlNonParsableResource = "Non-parsable SQL query"

// IdxSpanEffectiveDBStatement returns the db.statement the Datadog intake stores
// for span, a span of the idx tracer payload tp, and whether it stores one.
// agentVersion is the version of the agent that sent the payload
// (aggregator.TracePayload.AgentVersion), which the intake records on each span
// as _dd.agent_version.
//
// It emulates, without modifying the payload, the two steps the intake applies
// in order:
//  1. Remap: a db.statement sent by the agent is kept; otherwise the value of
//     sql.query, or failing that cassandra.query, becomes db.statement.
//  2. Backfill: if there is still no db.statement, the span type is sql or
//     cassandra, agentVersion is set, and the resource is neither empty nor
//     "Non-parsable SQL query", the resource becomes db.statement.
//
// The OTel db.query.* remap and the keep-old-tag options of the intake are not
// emulated.
func IdxSpanEffectiveDBStatement(tp *idx.TracerPayload, span *idx.Span, agentVersion string) (string, bool) {
	strs := tp.GetStrings()
	attrs := span.GetAttributes()
	for _, key := range []string{"db.statement", "sql.query", "cassandra.query"} {
		if v, ok := IdxStrAttr(strs, attrs, key); ok {
			return v, true
		}
	}
	switch IdxStr(strs, span.GetTypeRef()) {
	case "sql", "cassandra":
	default:
		return "", false
	}
	resource := IdxStr(strs, span.GetResourceRef())
	if agentVersion == "" || resource == "" || resource == sqlNonParsableResource {
		return "", false
	}
	return resource, true
}
