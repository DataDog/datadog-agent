// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package transform

import (
	semconv126 "go.opentelemetry.io/otel/semconv/v1.26.0"
	semconv "go.opentelemetry.io/otel/semconv/v1.6.1"

	"github.com/DataDog/datadog-agent/pkg/obfuscate"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
)

const (
	// TagRedisRawCommand represents a redis raw command tag
	TagRedisRawCommand = "redis.raw_command"
	// TagValkeyRawCommand represents a redis raw command tag
	TagValkeyRawCommand = "valkey.raw_command"
	// TagMemcachedCommand represents a memcached command tag
	TagMemcachedCommand = "memcached.command"
	// TagMongoDBQuery represents a MongoDB query tag
	TagMongoDBQuery = "mongodb.query"
	// TagElasticBody represents an Elasticsearch body tag
	TagElasticBody = "elasticsearch.body"
	// TagOpenSearchBody represents an OpenSearch body tag
	TagOpenSearchBody = "opensearch.body"
	// TagSQLQuery represents a SQL query tag
	TagSQLQuery = "sql.query"
	// TagHTTPURL represents an HTTP URL tag
	TagHTTPURL = "http.url"
	// TagDBMS represents a DBMS tag
	TagDBMS = "db.type"
)

const (
	// TextNonParsable is the error text used when a query is non-parsable
	TextNonParsable = "Non-parsable SQL query"
)

func obfuscateOTelDBAttributes(oq *obfuscate.ObfuscatedQuery, span *pb.Span) {
	if _, ok := traceutil.GetMeta(span, string(semconv.DBStatementKey)); ok {
		traceutil.SetMeta(span, string(semconv.DBStatementKey), oq.Query)
	}
	if _, ok := traceutil.GetMeta(span, string(semconv126.DBQueryTextKey)); ok {
		traceutil.SetMeta(span, string(semconv126.DBQueryTextKey), oq.Query)
	}
}

// ObfuscateSQLSpan obfuscates a SQL span using pkg/obfuscate logic
func ObfuscateSQLSpan(o *obfuscate.Obfuscator, span *pb.Span) (*obfuscate.ObfuscatedQuery, error) {
	if span.Resource == "" {
		return nil, nil
	}
	oq, err := o.ObfuscateSQLStringForDBMS(span.Resource, span.Meta[TagDBMS])
	if err != nil {
		// we have an error, discard the SQL to avoid polluting user resources.
		span.Resource = TextNonParsable
		traceutil.SetMeta(span, TagSQLQuery, TextNonParsable)
		return nil, err
	}
	span.Resource = oq.Query
	obfuscateOTelDBAttributes(oq, span)
	if len(oq.Metadata.TablesCSV) > 0 {
		traceutil.SetMeta(span, "sql.tables", oq.Metadata.TablesCSV)
	}
	traceutil.SetMeta(span, TagSQLQuery, oq.Query)
	return oq, nil
}

// ObfuscateRedisSpan obfuscates a Redis span using pkg/obfuscate logic
func ObfuscateRedisSpan(o *obfuscate.Obfuscator, span *pb.Span, removeAllArgs bool) {
	if span.Meta == nil || span.Meta[TagRedisRawCommand] == "" {
		return
	}
	if removeAllArgs {
		span.Meta[TagRedisRawCommand] = o.RemoveAllRedisArgs(span.Meta[TagRedisRawCommand])
		return
	}
	span.Meta[TagRedisRawCommand] = o.ObfuscateRedisString(span.Meta[TagRedisRawCommand])
}

// ObfuscateValkeySpan obfuscates a Valkey span using pkg/obfuscate logic
func ObfuscateValkeySpan(o *obfuscate.Obfuscator, span *pb.Span, removeAllArgs bool) {
	if span.Meta == nil || span.Meta[TagValkeyRawCommand] == "" {
		return
	}
	if removeAllArgs {
		span.Meta[TagValkeyRawCommand] = o.RemoveAllRedisArgs(span.Meta[TagValkeyRawCommand])
		return
	}
	span.Meta[TagValkeyRawCommand] = o.ObfuscateRedisString(span.Meta[TagValkeyRawCommand])
}
