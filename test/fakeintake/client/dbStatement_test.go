// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
)

// dbStatementTestSpan builds a one-span idx tracer payload with the given span
// type, resource and string attributes.
func dbStatementTestSpan(spanType, resource string, meta map[string]string) (*idx.TracerPayload, *idx.Span) {
	strs := []string{""}
	ref := func(s string) uint32 {
		if s == "" {
			return 0
		}
		strs = append(strs, s)
		return uint32(len(strs) - 1)
	}
	span := &idx.Span{
		TypeRef:     ref(spanType),
		ResourceRef: ref(resource),
		Attributes:  map[uint32]*idx.AnyValue{},
	}
	for k, v := range meta {
		span.Attributes[ref(k)] = strRef(ref(v))
	}
	return &idx.TracerPayload{Strings: strs}, span
}

func TestIdxSpanEffectiveDBStatement(t *testing.T) {
	const (
		agentVersion = "7.99.0"
		query        = "SELECT * FROM users WHERE id = ?"
		otherQuery   = "SELECT name FROM orders WHERE total > ?"
	)
	for _, tc := range []struct {
		name         string
		spanType     string
		resource     string
		meta         map[string]string
		agentVersion string
		want         string
		wantOK       bool
	}{
		{
			name:         "remap from sql.query",
			spanType:     "sql",
			resource:     query,
			meta:         map[string]string{"sql.query": otherQuery},
			agentVersion: agentVersion,
			want:         otherQuery,
			wantOK:       true,
		},
		{
			name:         "remap from cassandra.query",
			spanType:     "cassandra",
			resource:     query,
			meta:         map[string]string{"cassandra.query": otherQuery},
			agentVersion: agentVersion,
			want:         otherQuery,
			wantOK:       true,
		},
		{
			name:         "sql.query wins over cassandra.query",
			spanType:     "sql",
			resource:     query,
			meta:         map[string]string{"sql.query": otherQuery, "cassandra.query": query},
			agentVersion: agentVersion,
			want:         otherQuery,
			wantOK:       true,
		},
		{
			name:         "db.statement wins over cassandra.query",
			spanType:     "cassandra",
			resource:     query,
			meta:         map[string]string{"db.statement": otherQuery, "cassandra.query": query},
			agentVersion: agentVersion,
			want:         otherQuery,
			wantOK:       true,
		},
		{
			name:         "remap applies to any span type",
			spanType:     "web",
			resource:     "GET /",
			meta:         map[string]string{"sql.query": otherQuery},
			agentVersion: agentVersion,
			want:         otherQuery,
			wantOK:       true,
		},
		{
			name:         "existing db.statement kept over sql.query",
			spanType:     "sql",
			resource:     query,
			meta:         map[string]string{"db.statement": otherQuery, "sql.query": query},
			agentVersion: agentVersion,
			want:         otherQuery,
			wantOK:       true,
		},
		{
			name:         "backfill from resource on sql",
			spanType:     "sql",
			resource:     query,
			agentVersion: agentVersion,
			want:         query,
			wantOK:       true,
		},
		{
			name:         "backfill from resource on cassandra",
			spanType:     "cassandra",
			resource:     query,
			agentVersion: agentVersion,
			want:         query,
			wantOK:       true,
		},
		{
			name:         "no backfill for other span types",
			spanType:     "web",
			resource:     query,
			agentVersion: agentVersion,
		},
		{
			name:         "no backfill from empty resource",
			spanType:     "sql",
			agentVersion: agentVersion,
		},
		{
			name:         "no backfill from non-parsable resource",
			spanType:     "sql",
			resource:     "Non-parsable SQL query",
			agentVersion: agentVersion,
		},
		{
			name:     "no backfill without agent version",
			spanType: "sql",
			resource: query,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tp, span := dbStatementTestSpan(tc.spanType, tc.resource, tc.meta)
			strsBefore := append([]string(nil), tp.Strings...)
			attrsBefore := len(span.Attributes)

			got, ok := IdxSpanEffectiveDBStatement(tp, span, tc.agentVersion)
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.want, got)

			// The payload is left untouched.
			assert.Equal(t, strsBefore, tp.Strings)
			assert.Len(t, span.Attributes, attrsBefore)
		})
	}
}
