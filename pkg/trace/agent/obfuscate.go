// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/obfuscate"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-agent/pkg/trace/transform"
)

const (
	tagRedisRawCommand  = transform.TagRedisRawCommand
	tagMemcachedCommand = transform.TagMemcachedCommand
	tagMongoDBQuery     = transform.TagMongoDBQuery
	tagElasticBody      = transform.TagElasticBody
	tagOpenSearchBody   = transform.TagOpenSearchBody
	tagSQLQuery         = transform.TagSQLQuery
	tagHTTPURL          = transform.TagHTTPURL
)

const textNonParsable = transform.TextNonParsable

func (a *Agent) obfuscateSpan(span *pb.Span) {
	o := a.lazyInitObfuscator()

	if a.conf.Obfuscation != nil && a.conf.Obfuscation.CreditCards.Enabled {
		for k, v := range span.Meta {
			newV := o.ObfuscateCreditCardNumber(k, v)
			if v != newV {
				log.Debugf("obfuscating possible credit card under key %s from service %s", k, span.Service)
				span.Meta[k] = newV
			}
		}
	}

	switch span.Type {
	case "sql", "cassandra":
		oq, err := transform.ObfuscateSQLSpan(o, span)
		if err != nil {
			log.Debugf("Error parsing SQL query: %v. Resource: %q", err, span.Resource)
			return
		}
		if oq == nil {
			// no error was thrown but no query was found either
			return
		}
	case "redis":
		span.Resource = o.QuantizeRedisString(span.Resource)
		if a.conf.Obfuscation.Redis.Enabled {
			transform.ObfuscateRedisSpan(o, span, a.conf.Obfuscation.Redis.RemoveAllArgs)
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
	case "elasticsearch", "opensearch":
		if span.Meta == nil {
			return
		}
		if a.conf.Obfuscation.ES.Enabled {
			if span.Meta[tagElasticBody] != "" {
				span.Meta[tagElasticBody] = o.ObfuscateElasticSearchString(span.Meta[tagElasticBody])
			}
		}
		if a.conf.Obfuscation.OpenSearch.Enabled {
			if span.Meta[tagOpenSearchBody] != "" {
				span.Meta[tagOpenSearchBody] = o.ObfuscateOpenSearchString(span.Meta[tagOpenSearchBody])
			}
		}
	}
}

func (a *Agent) obfuscateStatsGroup(b *pb.ClientGroupedStats) {
	o := a.lazyInitObfuscator()

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

var (
	obfuscatorLock sync.Mutex
)

func (a *Agent) lazyInitObfuscator() *obfuscate.Obfuscator {
	// Ensure thread safe initialization
	obfuscatorLock.Lock()
	defer obfuscatorLock.Unlock()

	if a.obfuscator == nil {
		if a.obfuscatorConf != nil {
			a.obfuscator = obfuscate.NewObfuscator(*a.obfuscatorConf)
		} else {
			a.obfuscator = obfuscate.NewObfuscator(obfuscate.Config{})
		}
	}

	return a.obfuscator
}
