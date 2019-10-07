// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package obfuscate

import (
	"flag"
	"log"
	"os"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"

	"github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"
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
		log.Fatal(err)
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

	o = NewObfuscator(&Config{
		ES:    JSONSettings{},
		Mongo: JSONSettings{},
	})
	assert.Nil(o.es)
	assert.Nil(o.mongo)

	o = NewObfuscator(&Config{
		ES:    JSONSettings{Enabled: true},
		Mongo: JSONSettings{Enabled: true},
	})
	assert.NotNil(o.es)
	assert.NotNil(o.mongo)
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
		cfg *Config,
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
		&Config{Redis: true},
	))

	t.Run("redis/disabled", testConfig(
		"redis",
		"redis.raw_command",
		"SET key val",
		"SET key val",
		&Config{},
	))

	t.Run("http/enabled", testConfig(
		"http",
		"http.url",
		"http://mysite.mydomain/1/2?q=asd",
		"http://mysite.mydomain/?/??",
		&Config{
			RemovePathDigits:  true,
			RemoveQueryString: true,
		},
	))

	t.Run("http/disabled", testConfig(
		"http",
		"http.url",
		"http://mysite.mydomain/1/2?q=asd",
		"http://mysite.mydomain/1/2?q=asd",
		&Config{},
	))

	t.Run("web/enabled", testConfig(
		"web",
		"http.url",
		"http://mysite.mydomain/1/2?q=asd",
		"http://mysite.mydomain/?/??",
		&Config{
			RemovePathDigits:  true,
			RemoveQueryString: true,
		},
	))

	t.Run("web/disabled", testConfig(
		"web",
		"http.url",
		"http://mysite.mydomain/1/2?q=asd",
		"http://mysite.mydomain/1/2?q=asd",
		&Config{},
	))

	t.Run("json/enabled", testConfig(
		"elasticsearch",
		"elasticsearch.body",
		`{"role": "database"}`,
		`{"role":"?"}`,
		&Config{
			ES: JSONSettings{Enabled: true},
		},
	))

	t.Run("json/disabled", testConfig(
		"elasticsearch",
		"elasticsearch.body",
		`{"role": "database"}`,
		`{"role": "database"}`,
		&Config{},
	))

	t.Run("memcached/enabled", testConfig(
		"memcached",
		"memcached.command",
		"set key 0 0 0\r\nvalue",
		"set key 0 0 0",
		&Config{Memcached: true},
	))

	t.Run("memcached/disabled", testConfig(
		"memcached",
		"memcached.command",
		"set key 0 0 0 noreply\r\nvalue",
		"set key 0 0 0 noreply\r\nvalue",
		&Config{},
	))
}

func BenchmarkCompactWhitespaces(b *testing.B) {
	str := "a b       cde     fg       hi                     j  jk   lk lkjfdsalfd     afsd sfdafsd f"
	for i := 0; i < b.N; i++ {
		compactWhitespaces(str)
	}
}
