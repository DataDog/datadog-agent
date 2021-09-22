// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package obfuscate implements quantizing and obfuscating of tags and resources for
// a set of spans matching a certain criteria.
package obfuscate

import (
	"bytes"
	"sync/atomic"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

//go:generate easyjson -no_std_marshalers $GOFILE

// Obfuscator quantizes and obfuscates spans. The obfuscator is not safe for
// concurrent use.
type Obfuscator struct {
	opts                 *config.ObfuscationConfig
	es                   *jsonObfuscator // nil if disabled
	mongo                *jsonObfuscator // nil if disabled
	sqlExecPlan          *jsonObfuscator // nil if disabled
	sqlExecPlanNormalize *jsonObfuscator // nil if disabled
	creditCards          *ccObfuscator   // nil if disabled
	// sqlLiteralEscapes reports whether we should treat escape characters literally or as escape characters.
	// A non-zero value means 'yes'. Different SQL engines behave in different ways and the tokenizer needs
	// to be generic.
	// Not safe for concurrent use.
	sqlLiteralEscapes int32
	// queryCache keeps a cache of already obfuscated queries.
	queryCache *measuredCache
}

// SQLOptions holds options that change the behavior of the obfuscator for SQL.
// easyjson:json
type SQLOptions struct {
	// ReplaceDigits causes the obfuscator to replace digits in identifiers and table names with question marks.
	ReplaceDigits bool `json:"replace_digits"`
}

// SetSQLLiteralEscapes sets whether or not escape characters should be treated literally by the SQL obfuscator.
func (o *Obfuscator) SetSQLLiteralEscapes(ok bool) {
	if ok {
		atomic.StoreInt32(&o.sqlLiteralEscapes, 1)
	} else {
		atomic.StoreInt32(&o.sqlLiteralEscapes, 0)
	}
}

// SQLLiteralEscapes reports whether escape characters should be treated literally by the SQL obfuscator.
func (o *Obfuscator) SQLLiteralEscapes() bool {
	return atomic.LoadInt32(&o.sqlLiteralEscapes) == 1
}

// NewObfuscator creates a new obfuscator
func NewObfuscator(cfg *config.ObfuscationConfig) *Obfuscator {
	if cfg == nil {
		cfg = new(config.ObfuscationConfig)
	}
	o := Obfuscator{
		opts:       cfg,
		queryCache: newMeasuredCache(),
	}
	if cfg.ES.Enabled {
		o.es = newJSONObfuscator(&cfg.ES, &o)
	}
	if cfg.Mongo.Enabled {
		o.mongo = newJSONObfuscator(&cfg.Mongo, &o)
	}
	if cfg.SQLExecPlan.Enabled {
		o.sqlExecPlan = newJSONObfuscator(&cfg.SQLExecPlan, &o)
	}
	if cfg.SQLExecPlanNormalize.Enabled {
		o.sqlExecPlanNormalize = newJSONObfuscator(&cfg.SQLExecPlanNormalize, &o)
	}
	if cfg.CreditCards.Enabled {
		o.creditCards = newCreditCardsObfuscator(cfg.CreditCards.Luhn)
	}
	return &o
}

// Stop cleans up after a finished Obfuscator.
func (o *Obfuscator) Stop() { o.queryCache.Close() }

// Obfuscate may obfuscate span's properties based on its type and on the Obfuscator's
// configuration.
func (o *Obfuscator) Obfuscate(span *pb.Span) {
	switch span.Type {
	case "sql", "cassandra":
		o.obfuscateSQL(span)
	case "redis":
		o.quantizeRedis(span)
		if o.opts.Redis.Enabled {
			o.obfuscateRedis(span)
		}
	case "memcached":
		if o.opts.Memcached.Enabled {
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

// ObfuscateStatsGroup obfuscates the given stats bucket group.
func (o *Obfuscator) ObfuscateStatsGroup(b *pb.ClientGroupedStats) {
	switch b.Type {
	case "sql", "cassandra":
		oq, err := o.ObfuscateSQLString(b.Resource)
		if err != nil {
			log.Errorf("Error obfuscating stats group resource %q: %v", b.Resource, err)
			b.Resource = nonParsableResource
		} else {
			b.Resource = oq.Query
		}
	case "redis":
		b.Resource = o.QuantizeRedisString(b.Resource)
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

// replaceDigits replaces consecutive sequences of digits with '?',
// example: "jobs_2020_1597876964" --> "jobs_?_?"
func replaceDigits(buffer []byte) []byte {
	scanningDigit := false
	filtered := buffer[:0]
	for _, b := range buffer {
		// digits are encoded as 1 byte in utf8
		if isDigit(rune(b)) {
			if scanningDigit {
				continue
			}
			scanningDigit = true
			filtered = append(filtered, byte('?'))
			continue
		}
		scanningDigit = false
		filtered = append(filtered, b)
	}
	return filtered
}
