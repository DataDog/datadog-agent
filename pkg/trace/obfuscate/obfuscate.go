// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// Package obfuscate implements quantizing and obfuscating of tags and resources for
// a set of spans matching a certain criteria.
package obfuscate

import (
	"bytes"
	"sync/atomic"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
)

// Obfuscator quantizes and obfuscates spans. The obfuscator is not safe for
// concurrent use.
type Obfuscator struct {
	opts  *Config
	es    *jsonObfuscator // nil if disabled
	mongo *jsonObfuscator // nil if disabled
}

// Config specifies the obfuscator configuration.
type Config struct {
	// ES holds the obfuscation configuration for ElasticSearch bodies.
	ES JSONSettings

	// Mongo holds the obfuscation configuration for MongoDB queries.
	Mongo JSONSettings

	// RemoveQueryStrings specifies whether query strings should be removed from HTTP URLs.
	RemoveQueryString bool

	// RemovePathDigits specifies whether digits in HTTP path segments to be removed.
	RemovePathDigits bool

	// RemoveStackTraces specifies whether stack traces should be removed. More specifically,
	// the "error.stack" tag values will be cleared from spans.
	RemoveStackTraces bool

	// Redis enables obfuscatiion of the "redis.raw_command" tag for spans of type "redis".
	Redis bool

	// Redis enables obfuscatiion of the "memcached.command" tag for spans of type "memcached".
	Memcached bool

	// sqlLiteralEscapes reports whether we should treat escape characters literally or as escape characters.
	// A non-zero value means 'yes'. Different SQL engines behave in different ways and the tokenizer needs
	// to be generic.
	// Not safe for concurrent use.
	sqlLiteralEscapes int32
}

// SetSQLLiteralEscapes sets whether or not escape characters should be treated literally by the SQL obfuscator.
func (o *Obfuscator) SetSQLLiteralEscapes(ok bool) {
	if ok {
		atomic.StoreInt32(&o.opts.sqlLiteralEscapes, 1)
	} else {
		atomic.StoreInt32(&o.opts.sqlLiteralEscapes, 0)
	}
}

// SQLLiteralEscapes reports whether escape characters should be treated literally by the SQL obfuscator.
func (o *Obfuscator) SQLLiteralEscapes() bool {
	return atomic.LoadInt32(&o.opts.sqlLiteralEscapes) == 1
}

// NewObfuscator creates a new Obfuscator.
func NewObfuscator(cfg *Config) *Obfuscator {
	if cfg == nil {
		cfg = new(Config)
	}
	o := Obfuscator{opts: cfg}
	if cfg.ES.Enabled {
		o.es = newJSONObfuscator(&cfg.ES)
	}
	if cfg.Mongo.Enabled {
		o.mongo = newJSONObfuscator(&cfg.Mongo)
	}
	return &o
}

// Obfuscate may obfuscate span's properties based on its type and on the Obfuscator's
// configuration.
func (o *Obfuscator) Obfuscate(span *pb.Span) {
	switch span.Type {
	case "sql", "cassandra":
		o.obfuscateSQL(span)
	case "redis":
		o.quantizeRedis(span)
		if o.opts.Redis {
			o.obfuscateRedis(span)
		}
	case "memcached":
		if o.opts.Memcached {
			o.obfuscateMemcached(span)
		}
	case "web", "http":
		o.obfuscateHTTP(span)
	case "mongodb":
		o.obfuscateJSON(span, "mongodb.query", o.mongo)
	case "elasticsearch":
		o.obfuscateJSON(span, "elasticsearch.body", o.es)
	}
}

// compactWhitespaces compacts all whitespaces in t.
func compactWhitespaces(t string) string {
	n := len(t)
	r := make([]byte, n)
	spaceCode := uint8(32)
	isWhitespace := func(char uint8) bool { return char == spaceCode }
	nr := 0
	offset := 0
	for i := 0; i < n; i++ {
		if isWhitespace(t[i]) {
			copy(r[nr:], t[nr+offset:i])
			r[i-offset] = spaceCode
			nr = i + 1 - offset
			for j := i + 1; j < n; j++ {
				if !isWhitespace(t[j]) {
					offset += j - i - 1
					i = j
					break
				} else if j == n-1 {
					offset += j - i
					i = j
					break
				}
			}
		}
	}
	copy(r[nr:], t[nr+offset:n])
	r = r[:n-offset]
	return string(bytes.Trim(r, " "))
}
