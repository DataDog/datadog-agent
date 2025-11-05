// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"context"
	"testing"

	gzip "github.com/DataDog/datadog-agent/comp/trace/compression/impl-gzip"
	"github.com/DataDog/datadog-agent/pkg/obfuscate"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-go/v5/statsd"
)

func TestNewCreditCardsObfuscator(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := config.New()
	cfg.Endpoints[0].APIKey = "test"
	cfg.Obfuscation.CreditCards.Enabled = true
	a := NewAgent(ctx, cfg, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, gzip.NewComponent())
	assert.True(t, a.conf.Obfuscation.CreditCards.Enabled)
}

func TestObfuscateStatsGroup(t *testing.T) {
	statsGroup := func(typ, resource string) *pb.ClientGroupedStats {
		return &pb.ClientGroupedStats{
			Type:     typ,
			Resource: resource,
		}
	}
	for _, tt := range []struct {
		in  *pb.ClientGroupedStats // input stats
		out string                 // output obfuscated resource
	}{
		{statsGroup("sql", "SELECT 1 FROM db"), "SELECT ? FROM db"},
		{statsGroup("sql", "SELECT 1\nFROM Blogs AS [b\nORDER BY [b]"), textNonParsable},
		{statsGroup("redis", "ADD 1, 2"), "ADD"},
		{statsGroup("valkey", "ADD 1, 2"), "ADD"},
		{statsGroup("other", "ADD 1, 2"), "ADD 1, 2"},
	} {
		agnt, stop := agentWithDefaults()
		defer stop()
		agnt.obfuscateStatsGroup(tt.in)
		assert.Equal(t, tt.in.Resource, tt.out)
	}
}

// TestObfuscateDefaults ensures that running the obfuscator with no config continues to obfuscate/quantize
// SQL queries and Redis commands in span resources.
func TestObfuscateDefaults(t *testing.T) {
	t.Run("redis", func(t *testing.T) {
		cmd := "SET k v\nGET k"
		st := idx.NewStringTable()
		span := idx.NewInternalSpan(st, &idx.Span{
			TypeRef:     st.Add("redis"),
			ResourceRef: st.Add(cmd),
			Attributes: map[uint32]*idx.AnyValue{
				st.Add("redis.raw_command"): {
					Value: &idx.AnyValue_StringValueRef{
						StringValueRef: st.Add(cmd),
					},
				},
			},
		})
		agnt, stop := agentWithDefaults()
		defer stop()
		agnt.obfuscateSpanInternal(span)
		rawCmd, ok := span.GetAttributeAsString("redis.raw_command")
		assert.True(t, ok)
		assert.Equal(t, cmd, rawCmd)
		assert.Equal(t, "SET GET", span.Resource())
	})

	t.Run("valkey", func(t *testing.T) {
		cmd := "SET k v\nGET k"
		st := idx.NewStringTable()
		span := idx.NewInternalSpan(st, &idx.Span{
			TypeRef:     st.Add("valkey"),
			ResourceRef: st.Add(cmd),
			Attributes: map[uint32]*idx.AnyValue{
				st.Add("valkey.raw_command"): {
					Value: &idx.AnyValue_StringValueRef{
						StringValueRef: st.Add(cmd),
					},
				},
			},
		})
		agnt, stop := agentWithDefaults()
		defer stop()
		agnt.obfuscateSpanInternal(span)
		rawCmd, ok := span.GetAttributeAsString("valkey.raw_command")
		assert.True(t, ok)
		assert.Equal(t, cmd, rawCmd)
		assert.Equal(t, "SET GET", span.Resource())
	})

	t.Run("sql", func(t *testing.T) {
		query := "UPDATE users(name) SET ('Jim')"
		st := idx.NewStringTable()
		span := idx.NewInternalSpan(st, &idx.Span{
			TypeRef:     st.Add("sql"),
			ResourceRef: st.Add(query),
			Attributes: map[uint32]*idx.AnyValue{
				st.Add("sql.query"): {
					Value: &idx.AnyValue_StringValueRef{
						StringValueRef: st.Add(query),
					},
				},
			},
		})
		agnt, stop := agentWithDefaults()
		defer stop()
		agnt.obfuscateSpanInternal(span)
		sqlQuery, ok := span.GetAttributeAsString("sql.query")
		assert.True(t, ok)
		assert.Equal(t, "UPDATE users ( name ) SET ( ? )", sqlQuery)
		assert.Equal(t, "UPDATE users ( name ) SET ( ? )", span.Resource())
	})
}

func agentWithDefaults(features ...string) (agnt *Agent, stop func()) {
	ctx, cancelFunc := context.WithCancel(context.Background())
	cfg := config.New()
	for _, f := range features {
		cfg.Features[f] = struct{}{}
	}
	cfg.Endpoints[0].APIKey = "test"
	return NewAgent(ctx, cfg, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, gzip.NewComponent()), cancelFunc
}

func TestObfuscateConfig(t *testing.T) {
	// testConfig returns a test function which creates a span of type typ,
	// having a tag with key/val, runs the obfuscator on it using the given
	// configuration and asserts that the new tag value matches exp.
	testConfig := func(
		typ, key, val, exp string,
		ocfg *config.ObfuscationConfig,
	) func(*testing.T) {
		return func(t *testing.T) {
			ctx, cancelFunc := context.WithCancel(context.Background())
			cfg := config.New()
			cfg.Endpoints[0].APIKey = "test"
			cfg.Obfuscation = ocfg
			agnt := NewAgent(ctx, cfg, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, gzip.NewComponent())
			defer cancelFunc()
			st := idx.NewStringTable()
			span := idx.NewInternalSpan(st, &idx.Span{
				TypeRef:    st.Add(typ),
				Attributes: make(map[uint32]*idx.AnyValue),
			})
			// Use SetAttributeFromString to properly handle special fields like env, version, component
			span.SetAttributeFromString(key, val)
			agnt.obfuscateSpanInternal(span)
			result, ok := span.GetAttributeAsString(key)
			assert.True(t, ok)
			assert.Equal(t, exp, result)
		}
	}

	t.Run("redis/enabled", testConfig(
		"redis",
		"redis.raw_command",
		"SET key val",
		"SET key ?",
		&config.ObfuscationConfig{Redis: obfuscate.RedisConfig{Enabled: true}},
	))

	t.Run("redis/remove_all_args", testConfig(
		"redis",
		"redis.raw_command",
		"SET key val",
		"SET ?",
		&config.ObfuscationConfig{Redis: obfuscate.RedisConfig{
			Enabled:       true,
			RemoveAllArgs: true,
		}},
	))

	t.Run("redis/disabled", testConfig(
		"redis",
		"redis.raw_command",
		"SET key val",
		"SET key val",
		&config.ObfuscationConfig{},
	))

	t.Run("valkey/enabled", testConfig(
		"valkey",
		"valkey.raw_command",
		"SET key val",
		"SET key ?",
		&config.ObfuscationConfig{Valkey: obfuscate.ValkeyConfig{Enabled: true}},
	))

	t.Run("valkey/remove_all_args", testConfig(
		"valkey",
		"valkey.raw_command",
		"SET key val",
		"SET ?",
		&config.ObfuscationConfig{Valkey: obfuscate.ValkeyConfig{
			Enabled:       true,
			RemoveAllArgs: true,
		}},
	))

	t.Run("valkey/disabled", testConfig(
		"valkey",
		"valkey.raw_command",
		"SET key val",
		"SET key val",
		&config.ObfuscationConfig{},
	))

	t.Run("http/enabled", testConfig(
		"http",
		"http.url",
		"http://mysite.mydomain/1/2?q=asd",
		"http://mysite.mydomain/?/??",
		&config.ObfuscationConfig{HTTP: obfuscate.HTTPConfig{
			RemovePathDigits:  true,
			RemoveQueryString: true,
		}},
	))

	t.Run("http/disabled", testConfig(
		"http",
		"http.url",
		"http://mysite.mydomain/1/2?q=asd",
		"http://mysite.mydomain/1/2?q=asd",
		&config.ObfuscationConfig{},
	))

	t.Run("web/enabled", testConfig(
		"web",
		"http.url",
		"http://mysite.mydomain/1/2?q=asd",
		"http://mysite.mydomain/?/??",
		&config.ObfuscationConfig{HTTP: obfuscate.HTTPConfig{
			RemovePathDigits:  true,
			RemoveQueryString: true,
		}},
	))

	t.Run("web/disabled", testConfig(
		"web",
		"http.url",
		"http://mysite.mydomain/1/2?q=asd",
		"http://mysite.mydomain/1/2?q=asd",
		&config.ObfuscationConfig{},
	))

	t.Run("elasticsearch/enabled", testConfig(
		"elasticsearch",
		"elasticsearch.body",
		`{"role": "database"}`,
		`{"role":"?"}`,
		&config.ObfuscationConfig{
			ES: obfuscate.JSONConfig{Enabled: true},
		},
	))

	t.Run("elasticsearch/disabled", testConfig(
		"elasticsearch",
		"elasticsearch.body",
		`{"role": "database"}`,
		`{"role": "database"}`,
		&config.ObfuscationConfig{},
	))

	t.Run("opensearch/elasticsearch-type", testConfig(
		"elasticsearch",
		"opensearch.body",
		`{"role": "database"}`,
		`{"role":"?"}`,
		&config.ObfuscationConfig{
			OpenSearch: obfuscate.JSONConfig{Enabled: true},
		},
	))

	t.Run("opensearch/opensearch-type", testConfig(
		"opensearch",
		"opensearch.body",
		`{"role": "database"}`,
		`{"role":"?"}`,
		&config.ObfuscationConfig{
			OpenSearch: obfuscate.JSONConfig{Enabled: true},
		},
	))

	t.Run("opensearch/disabled", testConfig(
		"elasticsearch",
		"opensearch.body",
		`{"role": "database"}`,
		`{"role": "database"}`,
		&config.ObfuscationConfig{},
	))

	t.Run("memcached/enabled", testConfig(
		"memcached",
		"memcached.command",
		"set key 0 0 0\r\nvalue",
		"",
		&config.ObfuscationConfig{Memcached: obfuscate.MemcachedConfig{Enabled: true}},
	))

	t.Run("memcached/keep_command", testConfig(
		"memcached",
		"memcached.command",
		"set key 0 0 0\r\nvalue",
		"set key 0 0 0",
		&config.ObfuscationConfig{Memcached: obfuscate.MemcachedConfig{
			Enabled:     true,
			KeepCommand: true,
		}},
	))

	t.Run("memcached/disabled", testConfig(
		"memcached",
		"memcached.command",
		"set key 0 0 0 noreply\r\nvalue",
		"set key 0 0 0 noreply\r\nvalue",
		&config.ObfuscationConfig{},
	))

	t.Run("creditcard", func(t *testing.T) {
		for _, tt := range []struct {
			k, v string
			out  string
		}{
			// these tags are not even checked:
			{"error", "5105-1051-0510-5100", "5105-1051-0510-5100"},
			{"_dd.something", "5105-1051-0510-5100", "5105-1051-0510-5100"},
			{"env", "5105-1051-0510-5100", "5105-1051-0510-5100"},
			{"service", "5105-1051-0510-5100", "5105-1051-0510-5100"},
			{"version", "5105-1051-0510-5100", "5105-1051-0510-5100"},

			{"card.number", "5105", "5105"},
			{"card.number", "5105-1051-0510-5100", "?"},
		} {
			t.Run(tt.k, testConfig("generic",
				tt.k,
				tt.v,
				tt.out,
				&config.ObfuscationConfig{
					CreditCards: obfuscate.CreditCardsConfig{Enabled: true},
				}))
		}
	})
}

func TestSQLResourceQuery(t *testing.T) {
	assert := assert.New(t)
	agnt, stop := agentWithDefaults()
	defer stop()

	// Test case 1: span with only resource
	st1 := idx.NewStringTable()
	span1 := idx.NewInternalSpan(st1, &idx.Span{
		ResourceRef: st1.Add("SELECT * FROM users WHERE id = 42"),
		TypeRef:     st1.Add("sql"),
		Attributes:  make(map[uint32]*idx.AnyValue),
	})
	agnt.obfuscateSpanInternal(span1)
	assert.Equal("SELECT * FROM users WHERE id = ?", span1.Resource())
	sqlQuery1, ok1 := span1.GetAttributeAsString("sql.query")
	assert.True(ok1)
	assert.Equal("SELECT * FROM users WHERE id = ?", sqlQuery1)

	// Test case 2: span with resource and existing sql.query tag (ensure it gets overwritten with obfuscated value)
	st2 := idx.NewStringTable()
	span2 := idx.NewInternalSpan(st2, &idx.Span{
		ResourceRef: st2.Add("SELECT * FROM users WHERE id = 42"),
		TypeRef:     st2.Add("sql"),
		Attributes: map[uint32]*idx.AnyValue{
			st2.Add("sql.query"): {
				Value: &idx.AnyValue_StringValueRef{
					StringValueRef: st2.Add("SELECT * FROM users WHERE id = 42"),
				},
			},
		},
	})
	agnt.obfuscateSpanInternal(span2)
	assert.Equal("SELECT * FROM users WHERE id = ?", span2.Resource())
	sqlQuery2, ok2 := span2.GetAttributeAsString("sql.query")
	assert.True(ok2)
	assert.Equal("SELECT * FROM users WHERE id = ?", sqlQuery2)
}

func TestSQLResourceWithError(t *testing.T) {
	assert := assert.New(t)
	testCases := []struct {
		resource  string
		hasMeta   bool
		queryMeta string
	}{
		{
			resource:  "SELECT * FROM users WHERE id = '' AND '",
			hasMeta:   true,
			queryMeta: "SELECT * FROM users WHERE id = '' AND '",
		},
		{
			resource: "SELECT * FROM users WHERE id = '' AND '",
			hasMeta:  false,
		},
		{
			resource: "INSERT INTO pages (id, name) VALUES (%(id0)s, %(name0)s), (%(id1)s, %(name1",
			hasMeta:  false,
		},
		{
			resource: "INSERT INTO pages (id, name) VALUES (%(id0)s, %(name0)s), (%(id1)s, %(name1)",
			hasMeta:  false,
		},
		{
			resource: `SELECT [b].[BlogId], [b].[Name]
FROM [Blogs] AS [b
ORDER BY [b].[Name]`,
			hasMeta: false,
		},
	}

	agnt, stop := agentWithDefaults()
	defer stop()
	for _, tc := range testCases {
		st := idx.NewStringTable()
		attrs := make(map[uint32]*idx.AnyValue)
		if tc.hasMeta {
			attrs[st.Add("sql.query")] = &idx.AnyValue{
				Value: &idx.AnyValue_StringValueRef{
					StringValueRef: st.Add(tc.queryMeta),
				},
			}
		}
		span := idx.NewInternalSpan(st, &idx.Span{
			ResourceRef: st.Add(tc.resource),
			TypeRef:     st.Add("sql"),
			Attributes:  attrs,
		})
		agnt.obfuscateSpanInternal(span)
		assert.Equal("Non-parsable SQL query", span.Resource())
		sqlQuery, ok := span.GetAttributeAsString("sql.query")
		assert.True(ok)
		assert.Equal("Non-parsable SQL query", sqlQuery)
	}
}

func TestSQLTableNames(t *testing.T) {
	t.Run("on", func(t *testing.T) {
		st := idx.NewStringTable()
		span := idx.NewInternalSpan(st, &idx.Span{
			ResourceRef: st.Add("SELECT * FROM users WHERE id = 42"),
			TypeRef:     st.Add("sql"),
			Attributes:  make(map[uint32]*idx.AnyValue),
		})
		agnt, stop := agentWithDefaults("table_names")
		defer stop()
		agnt.obfuscateSpanInternal(span)
		tables, ok := span.GetAttributeAsString("sql.tables")
		assert.True(t, ok)
		assert.Equal(t, "users", tables)
	})

	t.Run("off", func(t *testing.T) {
		st := idx.NewStringTable()
		span := idx.NewInternalSpan(st, &idx.Span{
			ResourceRef: st.Add("SELECT * FROM users WHERE id = 42"),
			TypeRef:     st.Add("sql"),
			Attributes:  make(map[uint32]*idx.AnyValue),
		})
		agnt, stop := agentWithDefaults()
		defer stop()
		agnt.obfuscateSpanInternal(span)
		_, ok := span.GetAttributeAsString("sql.tables")
		assert.False(t, ok)
	})
}

func BenchmarkCCObfuscation(b *testing.B) {
	ctx, cancelFunc := context.WithCancel(context.Background())
	cfg := config.New()
	cfg.Endpoints[0].APIKey = "test"
	cfg.Obfuscation = &config.ObfuscationConfig{
		CreditCards: obfuscate.CreditCardsConfig{Enabled: true},
	}
	agnt := NewAgent(ctx, cfg, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, gzip.NewComponent())
	defer cancelFunc()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		st := idx.NewStringTable()
		span := idx.NewInternalSpan(st, &idx.Span{
			TypeRef: st.Add("typ"),
			Attributes: map[uint32]*idx.AnyValue{
				st.Add("akey"): {
					Value: &idx.AnyValue_StringValueRef{
						StringValueRef: st.Add("somestring"),
					},
				},
				st.Add("bkey"): {
					Value: &idx.AnyValue_StringValueRef{
						StringValueRef: st.Add("somestring"),
					},
				},
				st.Add("card.number"): {
					Value: &idx.AnyValue_StringValueRef{
						StringValueRef: st.Add("5105-1051-0510-5100"),
					},
				},
				st.Add("_sample_rate"): {
					Value: &idx.AnyValue_StringValueRef{
						StringValueRef: st.Add("1"),
					},
				},
				st.Add("sql.query"): {
					Value: &idx.AnyValue_StringValueRef{
						StringValueRef: st.Add("SELECT * FROM users WHERE id = 42"),
					},
				},
			},
		})
		agnt.obfuscateSpanInternal(span)
	}
}

func TestObfuscateSpanEvent(t *testing.T) {
	assert := assert.New(t)
	ctx, cancelFunc := context.WithCancel(context.Background())
	cfg := config.New()
	cfg.Endpoints[0].APIKey = "test"
	cfg.Obfuscation = &config.ObfuscationConfig{
		CreditCards: obfuscate.CreditCardsConfig{Enabled: true},
	}
	agnt := NewAgent(ctx, cfg, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, gzip.NewComponent())
	defer cancelFunc()

	spanEvent := &pb.SpanEvent{
		Name: "evt",
		Attributes: map[string]*pb.AttributeAnyValue{
			"str": {
				Type:        pb.AttributeAnyValue_STRING_VALUE,
				StringValue: "5105-1051-0510-5100",
			},
			"int": {
				Type:     pb.AttributeAnyValue_INT_VALUE,
				IntValue: 5105105105105100,
			},
			"dbl": {
				Type:        pb.AttributeAnyValue_DOUBLE_VALUE,
				DoubleValue: 5105105105105100,
			},
			"arr": {
				Type: pb.AttributeAnyValue_ARRAY_VALUE,
				ArrayValue: &pb.AttributeArray{
					Values: []*pb.AttributeArrayValue{
						{
							Type:        pb.AttributeArrayValue_STRING_VALUE,
							StringValue: "5105-1051-0510-5100",
						},
						{
							Type:     pb.AttributeArrayValue_INT_VALUE,
							IntValue: 5105105105105100,
						},
						{
							Type:        pb.AttributeArrayValue_DOUBLE_VALUE,
							DoubleValue: 5105105105105100,
						},
					},
				},
			},
		},
	}

	// Initialize the obfuscator (it's lazily initialized, so we need to trigger it first)
	// We can do this by creating a dummy span and calling obfuscateSpanInternal
	st := idx.NewStringTable()
	dummySpan := idx.NewInternalSpan(st, &idx.Span{
		TypeRef:    st.Add("dummy"),
		Attributes: make(map[uint32]*idx.AnyValue),
	})
	agnt.obfuscateSpanInternal(dummySpan)

	// Now test obfuscateSpanEvent
	agnt.obfuscateSpanEvent(spanEvent)

	for _, v := range spanEvent.Attributes {
		if v.Type == pb.AttributeAnyValue_ARRAY_VALUE {
			for _, arrayValue := range v.ArrayValue.Values {
				assert.Equal("?", arrayValue.StringValue)
			}
		} else {
			assert.Equal("?", v.StringValue)
		}
	}
}

func TestLexerObfuscation(t *testing.T) {
	ctx, cancelFunc := context.WithCancel(context.Background())
	cfg := config.New()
	cfg.Endpoints[0].APIKey = "test"
	cfg.Features["sqllexer"] = struct{}{}
	agnt := NewAgent(ctx, cfg, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, gzip.NewComponent())
	defer cancelFunc()
	st := idx.NewStringTable()
	span := idx.NewInternalSpan(st, &idx.Span{
		ResourceRef: st.Add("SELECT * FROM [u].[users]"),
		TypeRef:     st.Add("sql"),
		Attributes: map[uint32]*idx.AnyValue{
			st.Add("db.type"): {
				Value: &idx.AnyValue_StringValueRef{
					StringValueRef: st.Add("sqlserver"),
				},
			},
		},
	})
	agnt.obfuscateSpanInternal(span)
	assert.Equal(t, "SELECT * FROM [u].[users]", span.Resource())
}
