// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"context"
	"testing"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"

	"github.com/stretchr/testify/assert"
)

func TestNewCreditCardsObfuscator(t *testing.T) {
	_, ok := pb.MetaHook()
	assert.False(t, ok)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := config.New()
	cfg.Endpoints[0].APIKey = "test"
	cfg.Obfuscation.CreditCards.Enabled = true
	NewAgent(ctx, cfg, telemetry.NewNoopCollector())
	_, ok = pb.MetaHook()
	assert.True(t, ok)
}

func TestMetaHook(t *testing.T) {
	cco := newCreditCardsObfuscator(config.CreditCardsConfig{Enabled: true})
	defer cco.Stop()
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
		assert.Equal(t, tt.out, cco.MetaHook(tt.k, tt.v))
	}
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
		span := &pb.Span{
			Type:     "redis",
			Resource: cmd,
			Meta:     map[string]string{"redis.raw_command": cmd},
		}
		agnt, stop := agentWithDefaults()
		defer stop()
		agnt.obfuscateSpan(span)
		assert.Equal(t, cmd, span.Meta["redis.raw_command"])
		assert.Equal(t, "SET GET", span.Resource)
	})

	t.Run("sql", func(t *testing.T) {
		query := "UPDATE users(name) SET ('Jim')"
		span := &pb.Span{
			Type:     "sql",
			Resource: query,
			Meta:     map[string]string{"sql.query": query},
		}
		agnt, stop := agentWithDefaults()
		defer stop()
		agnt.obfuscateSpan(span)
		assert.Equal(t, "UPDATE users ( name ) SET ( ? )", span.Meta["sql.query"])
		assert.Equal(t, "UPDATE users ( name ) SET ( ? )", span.Resource)
	})
}

func agentWithDefaults(features ...string) (agnt *Agent, stop func()) {
	ctx, cancelFunc := context.WithCancel(context.Background())
	cfg := config.New()
	for _, f := range features {
		cfg.Features[f] = struct{}{}
	}
	cfg.Endpoints[0].APIKey = "test"
	return NewAgent(ctx, cfg, telemetry.NewNoopCollector()), cancelFunc
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
			agnt := NewAgent(ctx, cfg, telemetry.NewNoopCollector())
			defer cancelFunc()
			span := &pb.Span{Type: typ, Meta: map[string]string{key: val}}
			agnt.obfuscateSpan(span)
			assert.Equal(t, exp, span.Meta[key])
		}
	}

	t.Run("redis/enabled", testConfig(
		"redis",
		"redis.raw_command",
		"SET key val",
		"SET key ?",
		&config.ObfuscationConfig{Redis: config.RedisObfuscationConfig{Enabled: true}},
	))

	t.Run("redis/remove_all_args", testConfig(
		"redis",
		"redis.raw_command",
		"SET key val",
		"SET ?",
		&config.ObfuscationConfig{Redis: config.RedisObfuscationConfig{
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

	t.Run("http/enabled", testConfig(
		"http",
		"http.url",
		"http://mysite.mydomain/1/2?q=asd",
		"http://mysite.mydomain/?/??",
		&config.ObfuscationConfig{HTTP: config.HTTPObfuscationConfig{
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
		&config.ObfuscationConfig{HTTP: config.HTTPObfuscationConfig{
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

	t.Run("json/enabled", testConfig(
		"elasticsearch",
		"elasticsearch.body",
		`{"role": "database"}`,
		`{"role":"?"}`,
		&config.ObfuscationConfig{
			ES: config.JSONObfuscationConfig{Enabled: true},
		},
	))

	t.Run("json/disabled", testConfig(
		"elasticsearch",
		"elasticsearch.body",
		`{"role": "database"}`,
		`{"role": "database"}`,
		&config.ObfuscationConfig{},
	))

	t.Run("memcached/enabled", testConfig(
		"memcached",
		"memcached.command",
		"set key 0 0 0\r\nvalue",
		"set key 0 0 0",
		&config.ObfuscationConfig{Memcached: config.Enablable{Enabled: true}},
	))

	t.Run("memcached/disabled", testConfig(
		"memcached",
		"memcached.command",
		"set key 0 0 0 noreply\r\nvalue",
		"set key 0 0 0 noreply\r\nvalue",
		&config.ObfuscationConfig{},
	))
}

func SQLSpan(query string) *pb.Span {
	return &pb.Span{
		Resource: query,
		Type:     "sql",
		Meta: map[string]string{
			"sql.query": query,
		},
	}
}

func TestSQLResourceQuery(t *testing.T) {
	assert := assert.New(t)
	testCases := []*struct {
		span *pb.Span
	}{
		{
			&pb.Span{
				Resource: "SELECT * FROM users WHERE id = 42",
				Type:     "sql",
			},
		},
		{
			&pb.Span{
				Resource: "SELECT * FROM users WHERE id = 42",
				Type:     "sql",
				Meta: map[string]string{ // ensure that any existing sql.query tag gets overwritten with obfuscated value
					"sql.query": "SELECT * FROM users WHERE id = 42",
				},
			},
		},
	}

	agnt, stop := agentWithDefaults()
	defer stop()
	for _, tc := range testCases {
		agnt.obfuscateSpan(tc.span)
		assert.Equal("SELECT * FROM users WHERE id = ?", tc.span.Resource)
		assert.Equal("SELECT * FROM users WHERE id = ?", tc.span.Meta["sql.query"])
	}
}

func TestSQLResourceWithError(t *testing.T) {
	assert := assert.New(t)
	testCases := []*struct {
		span *pb.Span
	}{
		{
			&pb.Span{
				Resource: "SELECT * FROM users WHERE id = '' AND '",
				Type:     "sql",
				Meta: map[string]string{ // ensure that any existing sql.query tag gets overwritten with obfuscated value
					"sql.query": "SELECT * FROM users WHERE id = '' AND '",
				},
			},
		},
		{
			&pb.Span{
				Resource: "SELECT * FROM users WHERE id = '' AND '",
				Type:     "sql",
			},
		},
		{
			&pb.Span{
				Resource: "INSERT INTO pages (id, name) VALUES (%(id0)s, %(name0)s), (%(id1)s, %(name1",
				Type:     "sql",
			},
		},
		{
			&pb.Span{
				Resource: "INSERT INTO pages (id, name) VALUES (%(id0)s, %(name0)s), (%(id1)s, %(name1)",
				Type:     "sql",
			},
		},
		{
			&pb.Span{
				Resource: `SELECT [b].[BlogId], [b].[Name]
FROM [Blogs] AS [b
ORDER BY [b].[Name]`,
				Type: "sql",
			},
		},
	}

	agnt, stop := agentWithDefaults()
	defer stop()
	for _, tc := range testCases {
		agnt.obfuscateSpan(tc.span)
		assert.Equal("Non-parsable SQL query", tc.span.Resource)
		assert.Equal("Non-parsable SQL query", tc.span.Meta["sql.query"])
	}
}

func TestSQLTableNames(t *testing.T) {
	t.Run("on", func(t *testing.T) {
		span := &pb.Span{
			Resource: "SELECT * FROM users WHERE id = 42",
			Type:     "sql",
		}
		agnt, stop := agentWithDefaults("table_names")
		defer stop()
		agnt.obfuscateSpan(span)
		assert.Equal(t, "users", span.Meta["sql.tables"])
	})

	t.Run("off", func(t *testing.T) {
		span := &pb.Span{
			Resource: "SELECT * FROM users WHERE id = 42",
			Type:     "sql",
		}
		agnt, stop := agentWithDefaults()
		defer stop()
		agnt.obfuscateSpan(span)
		assert.Empty(t, span.Meta["sql.tables"])
	})
}
