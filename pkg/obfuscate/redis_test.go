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
			"MIGRATE host port key destination-db timeout",
			"MIGRATE ?",
		},
		{
			"MIGRATE host port key destination-db timeout COPY REPLACE",
			"MIGRATE ?",
		},
		{
			"MIGRATE host port \"\" destination-db timeout KEYS key1 key2 key3",
			"MIGRATE ?",
		},
		{
			"MIGRATE",
			"MIGRATE",
		},
		{
			"HELLO 3",
			"HELLO ?",
		},
		{
			"HELLO 3 AUTH username password",
			"HELLO ?",
		},
		{
			"HELLO 3 AUTH username password SETNAME clientname",
			"HELLO ?",
		},
		{
			"HELLO",
			"HELLO",
		},
		{
			"ACL SETUSER alice on >password ~* &* +@all",
			"ACL SETUSER ?",
		},
		{
			"ACL SETUSER bob on >mysecretpassword ~keys:* resetchannels &channel:* +@all -@dangerous",
			"ACL SETUSER ?",
		},
		{
			"ACL GETUSER alice",
			"ACL GETUSER ?",
		},
		{
			"ACL DELUSER alice",
			"ACL DELUSER ?",
		},
		{
			"ACL DELUSER alice bob charlie",
			"ACL DELUSER ?",
		},
		{
			"ACL LIST",
			"ACL LIST",
		},
		{
			"ACL WHOAMI",
			"ACL WHOAMI",
		},
		{
			"ACL",
			"ACL",
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
			"LPUSHX key value1 value2 value3",
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
			"RPUSHX key value1 value2 value3",
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
			"BITFIELD key SET i8 0 100 SET i8 8 200",
			"BITFIELD key SET i8 0 ? SET i8 8 ?",
		},
		{
			"BITFIELD key set i8 0 100 INCRBY i8 0 5 OVERFLOW SAT SET u4 4 7 GET u4 4",
			"BITFIELD key set i8 0 ? INCRBY i8 0 5 OVERFLOW SAT SET u4 4 ? GET u4 4",
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
			"HSET key field1 value field2 value",
			"HSET key field1 ? field2 ?",
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
			"zadd key nx score member",
			"zadd key nx score ?",
		},
		{
			"zadd key nx ch score member score member",
			"zadd key nx ch score ? score ?",
		},
		{
			"ZADD key GT CH score member score member",
			"ZADD key GT CH score ? score ?",
		},
		{
			"ZADD key XX LT score member",
			"ZADD key XX LT score ?",
		},
		{
			"CONFIG SET maxmemory 1gb requirepass s3cret",
			"CONFIG SET maxmemory ? requirepass ?",
		},
		{
			"config set p1 v1 p2 v2 p3 v3",
			"config set p1 ? p2 ? p3 ?",
		},
		{
			"CONFIG SET p1 v1 p2",
			"CONFIG SET p1 ? p2",
		},
		{
			"GEOADD key NX 13.36 38.11 member",
			"GEOADD key NX 13.36 38.11 ?",
		},
		{
			"GEOADD key XX CH 13.36 38.11 member1 15.08 37.50 member2",
			"GEOADD key XX CH 13.36 38.11 ? 15.08 37.50 ?",
		},
		{
			"geoadd key ch 13.36 38.11 member",
			"geoadd key ch 13.36 38.11 ?",
		},
		{
			"GEOADD key NX CH",
			"GEOADD key NX CH",
		},
		{
			"GEORADIUSBYMEMBER_RO key member 10 km WITHDIST",
			"GEORADIUSBYMEMBER_RO key ? 10 km WITHDIST",
		},
		{
			"georadiusbymember_ro key member 10 km",
			"georadiusbymember_ro key ? 10 km",
		},
		{
			"SMISMEMBER key member1 member2",
			"SMISMEMBER key ?",
		},
		{
			"smismember key member",
			"smismember key ?",
		},
		{
			"ZMSCORE key member1 member2",
			"ZMSCORE key ?",
		},
		{
			"zmscore key member",
			"zmscore key ?",
		},
		{
			"GEOSEARCH key FROMMEMBER member BYRADIUS 10 km ASC",
			"GEOSEARCH key FROMMEMBER ? BYRADIUS 10 km ASC",
		},
		{
			"geosearch key bybox 10 20 km frommember member",
			"geosearch key bybox 10 20 km frommember ?",
		},
		{
			"GEOSEARCH key FROMLONLAT 13.36 38.11 BYRADIUS 10 km",
			"GEOSEARCH key FROMLONLAT 13.36 38.11 BYRADIUS 10 km",
		},
		{
			"GEOSEARCHSTORE dest src FROMMEMBER member BYRADIUS 10 km STOREDIST",
			"GEOSEARCHSTORE dest src FROMMEMBER ? BYRADIUS 10 km STOREDIST",
		},
		{
			"GEOSEARCHSTORE frommember src FROMLONLAT 13.36 38.11 BYRADIUS 10 km",
			"GEOSEARCHSTORE frommember src FROMLONLAT 13.36 38.11 BYRADIUS 10 km",
		},
		{
			"GEOSEARCH key FROMMEMBER",
			"GEOSEARCH key FROMMEMBER",
		},
		{
			"SENTINEL SET mymaster auth-pass s3cret",
			"SENTINEL SET mymaster auth-pass ?",
		},
		{
			"sentinel set mymaster quorum 2 auth-pass s3cret",
			"sentinel set mymaster quorum ? auth-pass ?",
		},
		{
			"SENTINEL CONFIG SET sentinel-pass s3cret",
			"SENTINEL CONFIG SET sentinel-pass ?",
		},
		{
			"SENTINEL CONFIG SET resolve-hostnames yes sentinel-pass s3cret",
			"SENTINEL CONFIG SET resolve-hostnames ? sentinel-pass ?",
		},
		{
			"SENTINEL CONFIG GET sentinel-pass",
			"SENTINEL CONFIG GET sentinel-pass",
		},
		{
			"SENTINEL GET-MASTER-ADDR-BY-NAME mymaster",
			"SENTINEL GET-MASTER-ADDR-BY-NAME mymaster",
		},
		{
			"SENTINEL CONFIG",
			"SENTINEL CONFIG",
		},
		{
			"PUBLISH channel message",
			"PUBLISH channel ?",
		},
		{
			"spublish channel message",
			"spublish channel ?",
		},
		{
			"PUBLISH channel",
			"PUBLISH channel",
		},
		{
			"XADD stream * field value",
			"XADD stream * field ?",
		},
		{
			"XADD stream 1526919030474-0 f1 v1 f2 v2",
			"XADD stream 1526919030474-0 f1 ? f2 ?",
		},
		{
			"xadd stream nomkstream maxlen ~ 1000 limit 100 * field value",
			"xadd stream nomkstream maxlen ~ 1000 limit 100 * field ?",
		},
		{
			"XADD stream MINID 1526919030474 * field value",
			"XADD stream MINID 1526919030474 * field ?",
		},
		{
			"XADD stream MAXLEN = 10 * f1 v1 f2 v2",
			"XADD stream MAXLEN = 10 * f1 ? f2 ?",
		},
		{
			"XADD stream *",
			"XADD stream *",
		},
		{
			"XADD stream MAXLEN",
			"XADD stream MAXLEN",
		},
		{
			"LPOS key element",
			"LPOS key ?",
		},
		{
			"lpos key element RANK -1 COUNT 2 MAXLEN 100",
			"lpos key ? RANK -1 COUNT 2 MAXLEN 100",
		},
		{
			"HSETEX key FIELDS 1 field value",
			"HSETEX key FIELDS 1 field ?",
		},
		{
			"hsetex key fnx ex 60 fields 2 f1 v1 f2 v2",
			"hsetex key fnx ex 60 fields 2 f1 ? f2 ?",
		},
		{
			"HSETEX fields KEEPTTL FIELDS 1 field value",
			"HSETEX fields KEEPTTL FIELDS 1 field ?",
		},
		{
			"HSETEX key FIELDS 1 field",
			"HSETEX key FIELDS 1 field",
		},
		{
			"EVAL \"return redis.call('set', KEYS[1], ARGV[1])\" 1 key value",
			"EVAL \"return redis.call('set', KEYS[1], ARGV[1])\" 1 key ?",
		},
		{
			"EVALSHA 1b936e3fe509bcbc9cd0664897bbe8fd0ac95f9e 2 k1 k2 a1 a2",
			"EVALSHA 1b936e3fe509bcbc9cd0664897bbe8fd0ac95f9e 2 k1 k2 ?",
		},
		{
			"fcall myfunc 0 a1 a2",
			"fcall myfunc 0 ?",
		},
		{
			"FCALL_RO myfunc 1 key arg",
			"FCALL_RO myfunc 1 key ?",
		},
		{
			"EVAL_RO script 1 key arg",
			"EVAL_RO script 1 key ?",
		},
		{
			"EVALSHA_RO sha 1 key arg",
			"EVALSHA_RO sha 1 key ?",
		},
		{
			"EVAL script 2 k1 k2",
			"EVAL script 2 k1 k2",
		},
		{
			"EVAL script 9223372036854775807 k1 k2",
			"EVAL script 9223372036854775807 k1 k2",
		},
		{
			"EVAL script notanumber k1 a1",
			"EVAL script notanumber ?",
		},
		{
			"EVAL script -1 a1",
			"EVAL script -1 ?",
		},
		{
			"EVAL script 1",
			"EVAL script 1",
		},
		{
			"BITFIELD key SET i8 0 SET",
			"BITFIELD key SET i8 0 ?",
		},
		{
			"BITFIELD key SET i8 0 set SET u4 4 7",
			"BITFIELD key SET i8 0 ? SET u4 4 ?",
		},
		{
			"BITFIELD key SET i8 0 SET i8 0 100",
			"BITFIELD key SET i8 0 ? i8 0 ?",
		},
		{
			"ZRANGEBYLEX key [aaa (g LIMIT 0 10",
			"ZRANGEBYLEX key ? ? LIMIT 0 10",
		},
		{
			"zrevrangebylex key (g [aaa",
			"zrevrangebylex key ? ?",
		},
		{
			"ZLEXCOUNT key - +",
			"ZLEXCOUNT key ? ?",
		},
		{
			"ZREMRANGEBYLEX key [alpha [omega",
			"ZREMRANGEBYLEX key ? ?",
		},
		{
			"ZRANGEBYLEX key",
			"ZRANGEBYLEX key",
		},
		{
			"ZRANGE key [aaa (g BYLEX LIMIT 0 10",
			"ZRANGE key ? ? BYLEX LIMIT 0 10",
		},
		{
			"zrange key (g [aaa bylex rev",
			"zrange key ? ? bylex rev",
		},
		{
			"ZRANGE key 0 -1 WITHSCORES",
			"ZRANGE key 0 -1 WITHSCORES",
		},
		{
			"ZRANGE key 0 10 BYSCORE",
			"ZRANGE key 0 10 BYSCORE",
		},
		{
			"ZRANGESTORE dst src [aaa (g BYLEX",
			"ZRANGESTORE dst src ? ? BYLEX",
		},
		{
			"ZRANGESTORE dst src 0 -1",
			"ZRANGESTORE dst src 0 -1",
		},
		{
			"ZRANGE key bylex bylex",
			"ZRANGE key bylex bylex",
		},
		{
			"XADD stream KEEPREF * field value",
			"XADD stream KEEPREF * field ?",
		},
		{
			"xadd stream nomkstream delref maxlen ~ 10 * f1 v1 f2 v2",
			"xadd stream nomkstream delref maxlen ~ 10 * f1 ? f2 ?",
		},
		{
			"XADD stream ACKED MINID 0 * field value",
			"XADD stream ACKED MINID 0 * field ?",
		},
		{
			"XADD s IDMP p1 req-1 * f v",
			"XADD s IDMP p1 req-1 * f ?",
		},
		{
			"xadd s idmpauto p1 maxlen ~ 10 * f1 v1 f2 v2",
			"xadd s idmpauto p1 maxlen ~ 10 * f1 ? f2 ?",
		},
		{
			"XADD s UNKNOWNOPT * f v",
			"XADD s ?",
		},
		{
			"GEOSEARCH key FROMMEMBER alice FROMMEMBER secret BYRADIUS 10 km",
			"GEOSEARCH key FROMMEMBER ? FROMMEMBER ? BYRADIUS 10 km",
		},
		{
			"GEOSEARCHSTORE dest src frommember alice FROMMEMBER secret BYBOX 1 2 km",
			"GEOSEARCHSTORE dest src frommember ? FROMMEMBER ? BYBOX 1 2 km",
		},
		{
			"GEOSEARCH key FROMMEMBER FROMMEMBER BYRADIUS 10 km",
			"GEOSEARCH key FROMMEMBER ? BYRADIUS 10 km",
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

func TestRemoveAllRedisArgs(t *testing.T) {
	o := NewObfuscator(Config{})

	for ti, tt := range [...]struct {
		in, out string
	}{
		{
			"",
			"",
		},
		{
			"SET key value",
			"SET ?",
		},
		{
			"GET k",
			"GET ?",
		},
		{
			"FAKECMD key value hash",
			"FAKECMD ?",
		},
		{
			"AUTH password",
			"AUTH ?",
		},
		{
			"GET",
			"GET",
		},
		{
			"CONFIG SET key value",
			"CONFIG SET ?",
		},
		{
			"CONFIG GET key",
			"CONFIG GET ?",
		},
		{
			"CONFIG key",
			"CONFIG ?",
		},
		{
			"BITFIELD key SET key value GET key",
			"BITFIELD ? SET ? GET ?",
		},
		{
			"BITFIELD key INCRBY value",
			"BITFIELD ? INCRBY ?",
		},
		{
			"BITFIELD secret key",
			"BITFIELD ?",
		},
		{
			"set key value",
			"set ?",
		},
		{
			"Get key",
			"Get ?",
		},
		{
			"config key",
			"config ?",
		},
		{
			"CONFIG get key",
			"CONFIG get ?",
		},
		{
			"bitfield key SET key value incrby 3",
			"bitfield ? SET ? incrby ?",
		},
	} {
		t.Run(strconv.Itoa(ti), func(t *testing.T) {
			out := o.RemoveAllRedisArgs(tt.in)
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
