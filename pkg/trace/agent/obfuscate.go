// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/obfuscate"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
)

const (
	tagRedisRawCommand  = "redis.raw_command"
	tagMemcachedCommand = "memcached.command"
	tagMongoDBQuery     = "mongodb.query"
	tagElasticBody      = "elasticsearch.body"
	tagSQLQuery         = "sql.query"
	tagHTTPURL          = "http.url"
)

const (
	textNonParsable = "Non-parsable SQL query"
)

func (a *Agent) obfuscateSpan(span *pb.Span) {
	o := a.obfuscator
	switch span.Type {
	case "sql", "cassandra":
		if span.Resource == "" {
			return
		}
		oq, err := o.ObfuscateSQLString(span.Resource)
		if err != nil {
			// we have an error, discard the SQL to avoid polluting user resources.
			log.Debugf("Error parsing SQL query: %v. Resource: %q", err, span.Resource)
			span.Resource = textNonParsable
			traceutil.SetMeta(span, tagSQLQuery, textNonParsable)
			return
		}

		span.Resource = oq.Query
		if len(oq.Metadata.TablesCSV) > 0 {
			traceutil.SetMeta(span, "sql.tables", oq.Metadata.TablesCSV)
		}
		traceutil.SetMeta(span, tagSQLQuery, oq.Query)
	case "redis":
		span.Resource = o.QuantizeRedisString(span.Resource)
		if a.conf.Obfuscation.Redis.Enabled {
			if span.Meta == nil || span.Meta[tagRedisRawCommand] == "" {
				return
			}
			if a.conf.Obfuscation.Redis.RemoveAllArgs {
				span.Meta[tagRedisRawCommand] = o.RemoveAllRedisArgs(span.Meta[tagRedisRawCommand])
				return
			}
			span.Meta[tagRedisRawCommand] = o.ObfuscateRedisString(span.Meta[tagRedisRawCommand])
		}
	case "memcached":
		if !a.conf.Obfuscation.Memcached.Enabled {
			return
		}
		if span.Meta == nil || span.Meta[tagMemcachedCommand] == "" {
			return
		}
		span.Meta[tagMemcachedCommand] = o.ObfuscateMemcachedString(span.Meta[tagMemcachedCommand])
	case "web", "http":
		if span.Meta == nil || span.Meta[tagHTTPURL] == "" {
			return
		}
		span.Meta[tagHTTPURL] = o.ObfuscateURLString(span.Meta[tagHTTPURL])
	case "mongodb":
		if !a.conf.Obfuscation.Mongo.Enabled {
			return
		}
		if span.Meta == nil || span.Meta[tagMongoDBQuery] == "" {
			return
		}
		span.Meta[tagMongoDBQuery] = o.ObfuscateMongoDBString(span.Meta[tagMongoDBQuery])
	case "elasticsearch":
		if !a.conf.Obfuscation.ES.Enabled {
			return
		}
		if span.Meta == nil || span.Meta[tagElasticBody] == "" {
			return
		}
		span.Meta[tagElasticBody] = o.ObfuscateElasticSearchString(span.Meta[tagElasticBody])
	}
}

func (a *Agent) obfuscateStatsGroup(b *pb.ClientGroupedStats) {
	o := a.obfuscator
	switch b.Type {
	case "sql", "cassandra":
		oq, err := o.ObfuscateSQLString(b.Resource)
		if err != nil {
			log.Errorf("Error obfuscating stats group resource %q: %v", b.Resource, err)
			b.Resource = textNonParsable
		} else {
			b.Resource = oq.Query
		}
	case "redis":
		b.Resource = o.QuantizeRedisString(b.Resource)
	}
}

// ccObfuscator maintains credit card obfuscation state and processing.
type ccObfuscator struct {
	luhn bool
}

func newCreditCardsObfuscator(cfg config.CreditCardsConfig) *ccObfuscator {
	cco := &ccObfuscator{luhn: cfg.Luhn}
	if cfg.Enabled {
		// obfuscator disabled
		pb.SetMetaHook(cco.MetaHook)
	}
	return cco
}

func (cco *ccObfuscator) Stop() { pb.SetMetaHook(nil) }

// MetaHook checks the tag with the given key and val and returns the final
// value to be assigned to this tag.
//
// For example, in this specific use-case, if the val is detected to be a credit
// card number, "?" will be returned.
func (cco *ccObfuscator) MetaHook(k, v string) (newval string) {
	switch k {
	case "_sample_rate",
		"_sampling_priority_v1",
		"error",
		"error.msg",
		"error.type",
		"error.stack",
		"env",
		"graphql.field",
		"graphql.query",
		"graphql.type",
		"graphql.operation.name",
		"grpc.code",
		"grpc.method",
		"grpc.request",
		"http.status_code",
		"http.method",
		"runtime-id",
		"out.host",
		"out.port",
		"sampling.priority",
		"span.type",
		"span.name",
		"service.name",
		"service",
		"sql.query",
		"version":
		// these tags are known to not be credit card numbers
		return v
	}
	if strings.HasPrefix(k, "_dd") {
		return v
	}
	if obfuscate.IsCardNumber(v, cco.luhn) {
		return "?"
	}
	return v
}
