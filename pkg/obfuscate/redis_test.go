// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package obfuscate

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

type redisTestCase struct {
	query            string
	expectedResource string
}

func TestRedisQuantizer(t *testing.T) {
	assert := assert.New(t)
	o := NewObfuscator(Config{})

	queryToExpected := []redisTestCase{
		{"CLIENT",
			"CLIENT"}, // regression test for DataDog/datadog-trace-agent#421

		{"CLIENT LIST",
			"CLIENT LIST"},

		{"get my_key",
			"GET"},

		{"SET le_key le_value",
			"SET"},

		{"\n\n  \nSET foo bar  \n  \n\n  ",
			"SET"},

		{"CONFIG SET parameter value",
			"CONFIG SET"},

		{"SET toto tata \n \n  EXPIRE toto 15  ",
			"SET EXPIRE"},

		{"MSET toto tata toto tata toto tata \n ",
			"MSET"},

		{"MULTI\nSET k1 v1\nSET k2 v2\nSET k3 v3\nSET k4 v4\nDEL to_del\nEXEC",
			"MULTI SET SET ..."},

		{"DEL k1\nDEL k2\nHMSET k1 \"a\" 1 \"b\" 2 \"c\" 3\nHMSET k2 \"d\" \"4\" \"e\" \"4\"\nDEL k3\nHMSET k3 \"f\" \"5\"\nDEL k1\nDEL k2\nHMSET k1 \"a\" 1 \"b\" 2 \"c\" 3\nHMSET k2 \"d\" \"4\" \"e\" \"4\"\nDEL k3\nHMSET k3 \"f\" \"5\"\nDEL k1\nDEL k2\nHMSET k1 \"a\" 1 \"b\" 2 \"c\" 3\nHMSET k2 \"d\" \"4\" \"e\" \"4\"\nDEL k3\nHMSET k3 \"f\" \"5\"\nDEL k1\nDEL k2\nHMSET k1 \"a\" 1 \"b\" 2 \"c\" 3\nHMSET k2 \"d\" \"4\" \"e\" \"4\"\nDEL k3\nHMSET k3 \"f\" \"5\"",
			"DEL DEL HMSET ..."},

		{"GET...",
			"..."},

		{"GET k...",
			"GET"},

		{"GET k1\nGET k2\nG...",
			"GET GET ..."},

		{"GET k1\nGET k2\nDEL k3\nGET k...",
			"GET GET DEL ..."},

		{"GET k1\nGET k2\nHDEL k3 a\nG...",
			"GET GET HDEL ..."},

		{"GET k...\nDEL k2\nMS...",
			"GET DEL ..."},

		{"GET k...\nDE...\nMS...",
			"GET ..."},

		{"GET k1\nDE...\nGET k2",
			"GET GET"},

		{"GET k1\nDE...\nGET k2\nHDEL k3 a\nGET k4\nDEL k5",
			"GET GET HDEL ..."},

		{"UNKNOWN 123",
			"UNKNOWN"},
	}

	for _, testCase := range queryToExpected {
		out := o.QuantizeRedisString(testCase.query)
		assert.Equal(testCase.expectedResource, out)
	}
}

func TestRedisObfuscator(t *testing.T) {
	o := NewObfuscator(Config{})

	for ti, tt := range [...]struct {
		in, out string
	}{
		{
			"AUTH my-secret-password",
			"AUTH ?",
		},
		{
			"AUTH james my-secret-password",
			"AUTH ?",
		},
		{
			"AUTH",
			"AUTH",
		},
		{
			"APPEND key value",
			"APPEND key ?",
		},
		{
			"GETSET key value",
			"GETSET key ?",
		},
		{
			"LPUSHX key value",
			"LPUSHX key ?",
		},
		{
			"GEORADIUSBYMEMBER key member radius m|km|ft|mi [WITHCOORD] [WITHDIST] [WITHHASH] [COUNT count] [ASC|DESC] [STORE key] [STOREDIST key]",
			"GEORADIUSBYMEMBER key ? radius m|km|ft|mi [WITHCOORD] [WITHDIST] [WITHHASH] [COUNT count] [ASC|DESC] [STORE key] [STOREDIST key]",
		},
		{
			"RPUSHX key value",
			"RPUSHX key ?",
		},
		{
			"SET key value",
			"SET key ?",
		},
		{
			"SET key value [expiration EX seconds|PX milliseconds] [NX|XX]",
			"SET key ? [expiration EX seconds|PX milliseconds] [NX|XX]",
		},
		{
			"SETNX key value",
			"SETNX key ?",
		},
		{
			"SISMEMBER key member",
			"SISMEMBER key ?",
		},
		{
			"ZRANK key member",
			"ZRANK key ?",
		},
		{
			"ZREVRANK key member",
			"ZREVRANK key ?",
		},
		{
			"ZSCORE key member",
			"ZSCORE key ?",
		},
		{
			"BITFIELD key GET type offset SET type offset value INCRBY type",
			"BITFIELD key GET type offset SET type offset ? INCRBY type",
		},
		{
			"BITFIELD key SET type offset value INCRBY type",
			"BITFIELD key SET type offset ? INCRBY type",
		},
		{
			"BITFIELD key GET type offset INCRBY type",
			"BITFIELD key GET type offset INCRBY type",
		},
		{
			"BITFIELD key SET type offset",
			"BITFIELD key SET type offset",
		},
		{
			"CONFIG SET parameter value",
			"CONFIG SET parameter ?",
		},
		{
			"CONFIG foo bar baz",
			"CONFIG foo bar baz",
		},
		{
			"GEOADD key longitude latitude member longitude latitude member longitude latitude member",
			"GEOADD key longitude latitude ? longitude latitude ? longitude latitude ?",
		},
		{
			"GEOADD key longitude latitude member longitude latitude member",
			"GEOADD key longitude latitude ? longitude latitude ?",
		},
		{
			"GEOADD key longitude latitude member",
			"GEOADD key longitude latitude ?",
		},
		{
			"GEOADD key longitude latitude",
			"GEOADD key longitude latitude",
		},
		{
			"GEOADD key",
			"GEOADD key",
		},
		{
			"GEOHASH key\nGEOPOS key\n GEODIST key",
			"GEOHASH key\nGEOPOS key\nGEODIST key",
		},
		{
			"GEOHASH key member\nGEOPOS key member\nGEODIST key member\n",
			"GEOHASH key ?\nGEOPOS key ?\nGEODIST key ?",
		},
		{
			"GEOHASH key member member member\nGEOPOS key member member \n  GEODIST key member member member",
			"GEOHASH key ?\nGEOPOS key ?\nGEODIST key ?",
		},
		{
			"GEOPOS key member [member ...]",
			"GEOPOS key ?",
		},
		{
			"SREM key member [member ...]",
			"SREM key ?",
		},
		{
			"ZREM key member [member ...]",
			"ZREM key ?",
		},
		{
			"SADD key member [member ...]",
			"SADD key ?",
		},
		{
			"GEODIST key member1 member2 [unit]",
			"GEODIST key ?",
		},
		{
			"LPUSH key value [value ...]",
			"LPUSH key ?",
		},
		{
			"RPUSH key value [value ...]",
			"RPUSH key ?",
		},
		{
			"HSET key field value \nHSETNX key field value\nBLAH",
			"HSET key field ?\nHSETNX key field ?\nBLAH",
		},
		{
			"HSET key field value",
			"HSET key field ?",
		},
		{
			"HSETNX key field value",
			"HSETNX key field ?",
		},
		{
			"LREM key count value",
			"LREM key count ?",
		},
		{
			"LSET key index value",
			"LSET key index ?",
		},
		{
			"SETBIT key offset value",
			"SETBIT key offset ?",
		},
		{
			"SETRANGE key offset value",
			"SETRANGE key offset ?",
		},
		{
			"SETEX key seconds value",
			"SETEX key seconds ?",
		},
		{
			"PSETEX key milliseconds value",
			"PSETEX key milliseconds ?",
		},
		{
			"ZINCRBY key increment member",
			"ZINCRBY key increment ?",
		},
		{
			"SMOVE source destination member",
			"SMOVE source destination ?",
		},
		{
			"RESTORE key ttl serialized-value [REPLACE]",
			"RESTORE key ttl ? [REPLACE]",
		},
		{
			"LINSERT key BEFORE pivot value",
			"LINSERT key BEFORE pivot ?",
		},
		{
			"LINSERT key AFTER pivot value",
			"LINSERT key AFTER pivot ?",
		},
		{
			"HMSET key field value field value",
			"HMSET key field ? field ?",
		},
		{
			"HMSET key field value \n HMSET key field value\n\n ",
			"HMSET key field ?\nHMSET key field ?",
		},
		{
			"HMSET key field",
			"HMSET key field",
		},
		{
			"MSET key value key value",
			"MSET key ? key ?",
		},
		{
			"MSET\nMSET key value",
			"MSET\nMSET key ?",
		},
		{
			"MSET key value",
			"MSET key ?",
		},
		{
			"MSETNX key value key value",
			"MSETNX key ? key ?",
		},
		{
			"ZADD key score member score member",
			"ZADD key score ? score ?",
		},
		{
			"ZADD key NX score member score member",
			"ZADD key NX score ? score ?",
		},
		{
			"ZADD key NX CH score member score member",
			"ZADD key NX CH score ? score ?",
		},
		{
			"ZADD key NX CH INCR score member score member",
			"ZADD key NX CH INCR score ? score ?",
		},
		{
			"ZADD key XX INCR score member score member",
			"ZADD key XX INCR score ? score ?",
		},
		{
			"ZADD key XX INCR score member",
			"ZADD key XX INCR score ?",
		},
		{
			"ZADD key XX INCR score",
			"ZADD key XX INCR score",
		},
		{
			`
CONFIG command
SET k v
			`,
			`CONFIG command
SET k ?`,
		},
	} {
		t.Run(strconv.Itoa(ti), func(t *testing.T) {
			out := o.ObfuscateRedisString(tt.in)
			assert.Equal(t, tt.out, out, tt.in)
		})
	}
}

func BenchmarkRedisObfuscator(b *testing.B) {
	cmd := strings.Repeat("GEOADD key longitude latitude member longitude latitude member longitude latitude member\n", 5)
	o := NewObfuscator(Config{})
	b.Run(fmt.Sprintf("%db", len(cmd)), func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			o.ObfuscateRedisString(cmd)
		}
	})
}

func BenchmarkRedisQuantizer(b *testing.B) {
	b.ReportAllocs()
	cmd := `DEL k1\nDEL k2\nHMSET k1 "a" 1 "b" 2 "c" 3\nHMSET k2 "d" "4" "e" "4"\nDEL k3\nHMSET k3 "f" "5"\nDEL k1\nDEL k2\nHMSET k1 "a" 1 "b" 2 "c" 3\nHMSET k2 "d" "4" "e" "4"\nDEL k3\nHMSET k3 "f" "5"\nDEL k1\nDEL k2\nHMSET k1 "a" 1 "b" 2 "c" 3\nHMSET k2 "d" "4" "e" "4"\nDEL k3\nHMSET k3 "f" "5"\nDEL k1\nDEL k2\nHMSET k1 "a" 1 "b" 2 "c" 3\nHMSET k2 "d" "4" "e" "4"\nDEL k3\nHMSET k3 "f" "5"`
	o := NewObfuscator(Config{})

	for i := 0; i < b.N; i++ {
		o.QuantizeRedisString(cmd)
	}
}
