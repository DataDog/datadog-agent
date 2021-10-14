// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package obfuscate

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"

	"github.com/cihub/seelog"
	"github.com/mailru/easyjson/jlexer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type compactSpacesTestCase struct {
	before string
	after  string
}

func TestMain(m *testing.M) {
	flag.Parse()

	// disable loggging in tests
	seelog.UseLogger(seelog.Disabled)

	// prepare JSON obfuscator tests
	suite, err := loadTests()
	if err != nil {
		log.Fatalf("Failed to load JSON obfuscator tests: %s", err.Error())
	}
	if len(suite) == 0 {
		log.Fatal("no tests in suite")
	}
	jsonSuite = suite

	os.Exit(m.Run())
}

func TestNewObfuscator(t *testing.T) {
	assert := assert.New(t)
	o := NewObfuscator(nil)
	assert.Nil(o.es)
	assert.Nil(o.mongo)

	o = NewObfuscator(nil)
	assert.Nil(o.es)
	assert.Nil(o.mongo)
	assert.Nil(o.creditCards)

	_, ok := pb.MetaHook()
	assert.False(ok) // there is no hook without CC obfuscation on
	o = NewObfuscator(&config.ObfuscationConfig{
		ES:          config.JSONObfuscationConfig{Enabled: true},
		Mongo:       config.JSONObfuscationConfig{Enabled: true},
		CreditCards: config.CreditCardsConfig{Enabled: true},
	})
	defer o.Stop()
	assert.NotNil(o.creditCards)
	assert.NotNil(o.es)
	assert.NotNil(o.mongo)
	_, ok = pb.MetaHook()
	assert.True(ok)
}

func TestCompactWhitespaces(t *testing.T) {
	assert := assert.New(t)

	resultsToExpect := []compactSpacesTestCase{
		{"aa",
			"aa"},

		{" aa bb",
			"aa bb"},

		{"aa    bb  cc  dd ",
			"aa bb cc dd"},

		{"    ",
			""},

		{"a b       cde     fg       hi                     j  jk   lk lkjfdsalfd     afsd sfdafsd f",
			"a b cde fg hi j jk lk lkjfdsalfd afsd sfdafsd f"},

		{"   ¡™£¢∞§¶    •ªº–≠œ∑´®†¥¨ˆøπ “‘«åß∂ƒ©˙∆˚¬…æΩ≈ç√ ∫˜µ≤≥÷    ",
			"¡™£¢∞§¶ •ªº–≠œ∑´®†¥¨ˆøπ “‘«åß∂ƒ©˙∆˚¬…æΩ≈ç√ ∫˜µ≤≥÷"},
	}

	for _, testCase := range resultsToExpect {
		assert.Equal(testCase.after, compactWhitespaces(testCase.before))
	}
}

func TestReplaceDigits(t *testing.T) {
	assert := assert.New(t)

	for _, tt := range []struct {
		in       []byte
		expected []byte
	}{
		{
			[]byte("table123"),
			[]byte("table?"),
		},
		{
			[]byte(""),
			[]byte(""),
		},
		{
			[]byte("2020-table"),
			[]byte("?-table"),
		},
		{
			[]byte("sales_2019_07_01"),
			[]byte("sales_?_?_?"),
		},
		{
			[]byte("45"),
			[]byte("?"),
		},
	} {
		assert.Equal(tt.expected, replaceDigits(tt.in))
	}
}

func TestObfuscateStatsGroup(t *testing.T) {
	statsGroup := func(typ, resource string) *pb.ClientGroupedStats {
		return &pb.ClientGroupedStats{
			Type:     typ,
			Resource: resource,
		}
	}
	o := NewObfuscator(nil)
	for _, tt := range []struct {
		in  *pb.ClientGroupedStats // input stats
		out string                 // output obfuscated resource
	}{
		{statsGroup("sql", "SELECT 1 FROM db"), "SELECT ? FROM db"},
		{statsGroup("sql", "SELECT 1\nFROM Blogs AS [b\nORDER BY [b]"), nonParsableResource},
		{statsGroup("redis", "ADD 1, 2"), "ADD"},
		{statsGroup("other", "ADD 1, 2"), "ADD 1, 2"},
	} {
		o.ObfuscateStatsGroup(tt.in)
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
		NewObfuscator(nil).Obfuscate(span)
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
		NewObfuscator(nil).Obfuscate(span)
		assert.Equal(t, query, span.Meta["sql.query"])
		assert.Equal(t, "UPDATE users ( name ) SET ( ? )", span.Resource)
	})
}

func TestObfuscateConfig(t *testing.T) {
	// testConfig returns a test function which creates a span of type typ,
	// having a tag with key/val, runs the obfuscator on it using the given
	// configuration and asserts that the new tag value matches exp.
	testConfig := func(
		typ, key, val, exp string,
		cfg *config.ObfuscationConfig,
	) func(*testing.T) {
		return func(t *testing.T) {
			span := &pb.Span{Type: typ, Meta: map[string]string{key: val}}
			NewObfuscator(cfg).Obfuscate(span)
			assert.Equal(t, exp, span.Meta[key])
		}
	}

	t.Run("redis/enabled", testConfig(
		"redis",
		"redis.raw_command",
		"SET key val",
		"SET key ?",
		&config.ObfuscationConfig{Redis: config.Enablable{Enabled: true}},
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

// TestSQLObfuscationOptionsDeserializationMethod checks if the use of easyjson results in the same deserialization
// output as encoding/json.
func TestSQLObfuscationOptionsDeserializationMethod(t *testing.T) {
	opts, err := json.Marshal(SQLOptions{ReplaceDigits: true})
	require.NoError(t, err)

	var in, out SQLOptions

	err = json.Unmarshal(opts, &in)
	require.NoError(t, err)

	jl := &jlexer.Lexer{Data: opts}
	out.UnmarshalEasyJSON(jl)
	require.NoError(t, jl.Error())

	assert.Equal(t, in, out)
}

func BenchmarkSQLObfuscationOptionsEasyJSONDeserialization(b *testing.B) {
	for i := 1; i <= 100000; i *= 10 {
		b.Run(fmt.Sprintf("Range:%d", i), func(b *testing.B) {
			benchmarkSQLObfuscationOptionsEasyJSONDeserialization(b)
		})
	}
}

func benchmarkSQLObfuscationOptionsEasyJSONDeserialization(b *testing.B) {
	b.ReportAllocs()
	opts, err := json.Marshal(SQLOptions{ReplaceDigits: true})
	require.NoError(b, err)
	for i := 0; i < b.N; i++ {
		var sqlCfg SQLOptions
		jl := &jlexer.Lexer{Data: opts}
		sqlCfg.UnmarshalEasyJSON(jl)
		require.NoError(b, jl.Error())
	}
}
func BenchmarkSQLObfuscationOptionsRegularJSONDeserialization(b *testing.B) {
	for i := 1; i <= 100000; i *= 10 {
		b.Run(fmt.Sprintf("Range:%d", i), func(b *testing.B) {
			benchmarkSQLObfuscationOptionsRegularJSONDeserialization(b)
		})
	}
}

func benchmarkSQLObfuscationOptionsRegularJSONDeserialization(b *testing.B) {
	b.ReportAllocs()
	opts, err := json.Marshal(SQLOptions{ReplaceDigits: true})
	require.NoError(b, err)
	for i := 0; i < b.N; i++ {
		var sqlCfg SQLOptions
		err := json.Unmarshal(opts, &sqlCfg)
		require.NoError(b, err)
	}
}

func TestLiteralEscapes(t *testing.T) {
	o := NewObfuscator(nil)

	t.Run("default", func(t *testing.T) {
		assert.False(t, o.SQLLiteralEscapes())
	})

	t.Run("true", func(t *testing.T) {
		o.SetSQLLiteralEscapes(true)
		assert.True(t, o.SQLLiteralEscapes())
	})

	t.Run("false", func(t *testing.T) {
		o.SetSQLLiteralEscapes(false)
		assert.False(t, o.SQLLiteralEscapes())
	})
}

func BenchmarkCompactWhitespaces(b *testing.B) {
	str := "a b       cde     fg       hi                     j  jk   lk lkjfdsalfd     afsd sfdafsd f"
	for i := 0; i < b.N; i++ {
		compactWhitespaces(str)
	}
}

func BenchmarkReplaceDigits(b *testing.B) {
	tbl := []byte("sales_2019_07_01_orders")
	for i := 0; i < b.N; i++ {
		replaceDigits(tbl)
	}
}
