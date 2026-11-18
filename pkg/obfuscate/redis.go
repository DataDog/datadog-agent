// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package obfuscate

import (
	"strconv"
	"strings"
)

// redisTruncationMark is used as suffix by tracing libraries to indicate that a
// command was truncated.
const redisTruncationMark = "..."

const maxRedisNbCommands = 3

// Redis commands consisting in 2 words
var redisCompoundCommandSet = map[string]bool{
	"CLIENT": true, "CLUSTER": true, "COMMAND": true, "CONFIG": true, "DEBUG": true, "SCRIPT": true,
}

// QuantizeRedisString returns a quantized version of a Redis query.
//
// TODO(gbbr): Refactor this method to use the tokenizer and
// remove "compactWhitespaces". This method is buggy when commands
// contain quoted strings with newlines.
func (*Obfuscator) QuantizeRedisString(query string) string {
	query = compactWhitespaces(query)

	var resource strings.Builder
	truncated := false
	nbCmds := 0

	for len(query) > 0 && nbCmds < maxRedisNbCommands {
		var rawLine string

		// Read the next command
		idx := strings.IndexByte(query, '\n')
		if idx == -1 {
			rawLine = query
			query = ""
		} else {
			rawLine = query[:idx]
			query = query[idx+1:]
		}

		line := strings.Trim(rawLine, " ")
		if len(line) == 0 {
			continue
		}

		// Parse arguments
		args := strings.SplitN(line, " ", 3)

		if strings.HasSuffix(args[0], redisTruncationMark) {
			truncated = true
			continue
		}

		command := strings.ToUpper(args[0])

		if redisCompoundCommandSet[command] && len(args) > 1 {
			if strings.HasSuffix(args[1], redisTruncationMark) {
				truncated = true
				continue
			}

			command += " " + strings.ToUpper(args[1])
		}

		// Write the command representation
		resource.WriteByte(' ')
		resource.WriteString(command)

		nbCmds++
		truncated = false
	}

	if nbCmds == maxRedisNbCommands || truncated {
		resource.WriteString(" ...")
	}

	return strings.Trim(resource.String(), " ")
}

// ObfuscateRedisString obfuscates the given Redis command.
func (*Obfuscator) ObfuscateRedisString(rediscmd string) string {
	t := newRedisTokenizer([]byte(rediscmd))
	var (
		str  strings.Builder
		cmd  string
		args []string
	)
	for {
		tok, typ, done := t.scan()
		switch typ {
		case redisTokenCommand:
			// new command starting
			if cmd != "" {
				// a previous command was buffered, obfuscate it
				obfuscateRedisCmd(&str, cmd, args...)
				str.WriteByte('\n')
			}
			cmd = tok
			args = args[:0]
		case redisTokenArgument:
			args = append(args, tok)
		}
		if done {
			// last command
			obfuscateRedisCmd(&str, cmd, args...)
			break
		}
	}
	return str.String()
}

func obfuscateRedisCmd(out *strings.Builder, cmd string, args ...string) {
	out.WriteString(cmd)
	if len(args) == 0 {
		return
	}
	out.WriteByte(' ')

	switch strings.ToUpper(cmd) {
	case "AUTH", "MIGRATE", "HELLO":
		// Obfuscate everything after command
		// • AUTH password
		// • MIGRATE host port key|"" destination-db timeout [COPY] [REPLACE] [AUTH password] [AUTH2 username password] [KEYS key [key ...]]
		// • HELLO [protover [AUTH username password] [SETNAME clientname]]
		if len(args) > 0 {
			args[0] = "?"
			args = args[:1]
		}

	case "ACL":
		// Obfuscate all arguments after the subcommand
		// • ACL SETUSER username on >password ~keys &channels +commands
		// • ACL GETUSER username
		// • ACL DELUSER username [username ...]
		// • ACL LIST
		// • ACL WHOAMI
		if len(args) > 1 {
			args[1] = "?"
			args = args[:2]
		}

	case "APPEND", "GETSET", "LPOS", "GEORADIUSBYMEMBER", "GEORADIUSBYMEMBER_RO",
		"PUBLISH", "SET", "SETNX", "SISMEMBER", "SPUBLISH", "ZRANK", "ZREVRANK", "ZSCORE":
		// Obfuscate 2nd argument:
		// • APPEND key value
		// • GETSET key value
		// • LPOS key element [RANK rank] [COUNT num-matches] [MAXLEN len]
		// • GEORADIUSBYMEMBER key member radius m|km|ft|mi [WITHCOORD] [WITHDIST] [WITHHASH] [COUNT count] [ASC|DESC] [STORE key] [STOREDIST key]
		// • GEORADIUSBYMEMBER_RO key member radius m|km|ft|mi [WITHCOORD] [WITHDIST] [WITHHASH] [COUNT count] [ASC|DESC]
		// • PUBLISH channel message
		// • SET key value [expiration EX seconds|PX milliseconds] [NX|XX]
		// • SETNX key value
		// • SISMEMBER key member
		// • SPUBLISH shardchannel message
		// • ZRANK key member
		// • ZREVRANK key member
		// • ZSCORE key member
		obfuscateRedisArgN(args, 1)

	case "HSETNX", "LREM", "LSET", "SETBIT", "SETEX", "PSETEX",
		"SETRANGE", "ZINCRBY", "SMOVE", "RESTORE":
		// Obfuscate 3rd argument:
		// • HSETNX key field value
		// • LREM key count value
		// • LSET key index value
		// • SETBIT key offset value
		// • SETEX key seconds value
		// • PSETEX key milliseconds value
		// • SETRANGE key offset value
		// • ZINCRBY key increment member
		// • SMOVE source destination member
		// • RESTORE key ttl serialized-value [REPLACE]
		obfuscateRedisArgN(args, 2)

	case "LINSERT":
		// Obfuscate 4th argument:
		// • LINSERT key BEFORE|AFTER pivot value
		obfuscateRedisArgN(args, 3)

	case "GEOHASH", "GEOPOS", "GEODIST", "LPUSH", "LPUSHX", "RPUSH",
		"RPUSHX", "SREM", "ZREM", "SADD", "SMISMEMBER", "ZMSCORE":
		// Obfuscate all arguments after the first one.
		// • GEOHASH key member [member ...]
		// • GEOPOS key member [member ...]
		// • GEODIST key member1 member2 [unit]
		// • LPUSH key value [value ...]
		// • LPUSHX key value [value ...]
		// • RPUSH key value [value ...]
		// • RPUSHX key value [value ...]
		// • SMISMEMBER key member [member ...]
		// • SREM key member [member ...]
		// • ZREM key member [member ...]
		// • SADD key member [member ...]
		// • ZMSCORE key member [member ...]
		if len(args) > 1 {
			args[1] = "?"
			args = args[:2]
		}

	case "GEOADD":
		// Obfuscating every 3rd argument after potential optional ones.
		// • GEOADD key [NX|XX] [CH] longitude latitude member [longitude latitude member ...]
		i := 1
	geoaddOptions:
		for ; i < len(args); i++ {
			switch strings.ToUpper(args[i]) {
			case "NX", "XX", "CH":
				// continue
			default:
				break geoaddOptions
			}
		}
		obfuscateRedisArgsStep(args, i, 3)

	case "GEOSEARCH", "GEOSEARCHSTORE":
		// Obfuscate the argument following every FROMMEMBER.
		// • GEOSEARCH key <FROMMEMBER member | FROMLONLAT longitude latitude> <BYRADIUS radius unit | BYBOX width height unit> [ASC|DESC] [COUNT count [ANY]] [WITHCOORD] [WITHDIST] [WITHHASH]
		// • GEOSEARCHSTORE destination source <FROMMEMBER member | FROMLONLAT longitude latitude> <BYRADIUS radius unit | BYBOX width height unit> [ASC|DESC] [COUNT count [ANY]] [STOREDIST]
		start := 1 // skip key
		if strings.ToUpper(cmd) == "GEOSEARCHSTORE" {
			start = 2 // skip destination and source
		}
		for i := start; i < len(args)-1; i++ {
			if strings.ToUpper(args[i]) == "FROMMEMBER" {
				i++
				args[i] = "?"
			}
		}

	case "HSET", "HMSET":
		// Every 2nd argument starting from first.
		// • HSET key field value [field value ...]
		// • HMSET key field value [field value ...]
		obfuscateRedisArgsStep(args, 1, 2)

	case "XADD":
		// Every 2nd argument after the entry ID, skipping potential optional ones.
		// • XADD key [NOMKSTREAM] [KEEPREF|DELREF|ACKED] [IDMPAUTO producer-id|IDMP producer-id idempotent-id] [MAXLEN|MINID [=|~] threshold [LIMIT count]] *|id field value [field value ...]
		i := 1
	xaddOptions:
		for i < len(args) {
			switch strings.ToUpper(args[i]) {
			case "NOMKSTREAM", "KEEPREF", "DELREF", "ACKED":
				i++
			case "IDMPAUTO":
				i += 2 // producer-id
			case "IDMP":
				i += 3 // producer-id idempotent-id
			case "MAXLEN", "MINID":
				i++
				if i < len(args) && (args[i] == "=" || args[i] == "~") {
					i++
				}
				i++ // threshold
			case "LIMIT":
				i += 2
			default:
				break xaddOptions
			}
		}
		// args[i] is the entry ID
		if i < len(args) && (len(args)-i-1)%2 != 0 {
			// Field/value pairs are misaligned, likely due to an unknown option:
			// fail closed by obfuscating everything after the key.
			args[1] = "?"
			args = args[:2]
			break
		}
		obfuscateRedisArgsStep(args, i+1, 2)

	case "HSETEX":
		// Every 2nd argument after the FIELDS numfields block.
		// • HSETEX key [FNX|FXX] [EX seconds|PX milliseconds|EXAT unix-time-seconds|PXAT unix-time-milliseconds|KEEPTTL] FIELDS numfields field value [field value ...]
		for i := 1; i < len(args); i++ {
			if strings.ToUpper(args[i]) == "FIELDS" {
				obfuscateRedisArgsStep(args, i+2, 2)
				break
			}
		}

	case "EVAL", "EVAL_RO", "EVALSHA", "EVALSHA_RO", "FCALL", "FCALL_RO":
		// Obfuscate all arguments after the keys.
		// • EVAL script numkeys [key [key ...]] [arg [arg ...]]
		// • EVAL_RO script numkeys [key [key ...]] [arg [arg ...]]
		// • EVALSHA sha1 numkeys [key [key ...]] [arg [arg ...]]
		// • EVALSHA_RO sha1 numkeys [key [key ...]] [arg [arg ...]]
		// • FCALL function numkeys [key [key ...]] [arg [arg ...]]
		// • FCALL_RO function numkeys [key [key ...]] [arg [arg ...]]
		if len(args) < 3 {
			break
		}
		start := 2
		if numkeys, err := strconv.Atoi(args[1]); err == nil && numkeys > 0 {
			// clamp numkeys so that a huge value can't overflow start
			start += min(numkeys, len(args))
		}
		if start < len(args) {
			args[start] = "?"
			args = args[:start+1]
		}

	case "MSET", "MSETNX":
		// Every 2nd argument starting from command.
		// • MSET key value [key value ...]
		// • MSETNX key value [key value ...]
		obfuscateRedisArgsStep(args, 0, 2)

	case "CONFIG":
		// Obfuscate every value of the SET sub-command.
		// • CONFIG SET parameter value [parameter value ...]
		if strings.ToUpper(args[0]) == "SET" {
			obfuscateRedisArgsStep(args, 1, 2)
		}

	case "SENTINEL":
		// Obfuscate every value of the SET and CONFIG SET sub-commands.
		// • SENTINEL SET master-name option value [option value ...]
		// • SENTINEL CONFIG SET parameter value [parameter value ...]
		switch strings.ToUpper(args[0]) {
		case "SET":
			obfuscateRedisArgsStep(args, 2, 2)
		case "CONFIG":
			if len(args) > 1 && strings.ToUpper(args[1]) == "SET" {
				obfuscateRedisArgsStep(args, 2, 2)
			}
		}

	case "BITFIELD":
		// Obfuscate 3rd argument to every SET sub-command:
		// • BITFIELD key [GET type offset] [SET type offset value] [INCRBY type offset increment] [OVERFLOW WRAP|SAT|FAIL]
		var n int
		for i, arg := range args {
			if n > 0 && i-n == 3 {
				args[i] = "?"
				n = 0
				// no continue: arg still holds the original token, so a masked
				// "SET" value is still recognized as the next SET sub-command.
			}
			if strings.ToUpper(arg) == "SET" {
				n = i
			}
		}

	case "ZRANGEBYLEX", "ZREVRANGEBYLEX", "ZLEXCOUNT", "ZREMRANGEBYLEX":
		// Obfuscate the lexicographical range, made of member values.
		// • ZRANGEBYLEX key min max [LIMIT offset count]
		// • ZREVRANGEBYLEX key max min [LIMIT offset count]
		// • ZLEXCOUNT key min max
		// • ZREMRANGEBYLEX key min max
		obfuscateRedisArgN(args, 1)
		obfuscateRedisArgN(args, 2)

	case "ZRANGE", "ZRANGESTORE":
		// Obfuscate the range when it is lexicographical.
		// • ZRANGE key start stop [BYSCORE|BYLEX] [REV] [LIMIT offset count] [WITHSCORES]
		// • ZRANGESTORE dst src min max [BYSCORE|BYLEX] [REV] [LIMIT offset count]
		start := 1 // skip key
		if strings.ToUpper(cmd) == "ZRANGESTORE" {
			start = 2 // skip dst and src
		}
		for i := start + 2; i < len(args); i++ {
			if strings.ToUpper(args[i]) == "BYLEX" {
				obfuscateRedisArgN(args, start)
				obfuscateRedisArgN(args, start+1)
				break
			}
		}

	case "ZADD":
		// Obfuscate every 2nd argument after potential optional ones.
		// • ZADD key [NX|XX] [GT|LT] [CH] [INCR] score member [score member ...]
		var i int
	loop:
		for i = range args {
			if i == 0 {
				continue // key
			}
			switch strings.ToUpper(args[i]) {
			case "NX", "XX", "GT", "LT", "CH", "INCR":
				// continue
			default:
				break loop
			}
		}
		obfuscateRedisArgsStep(args, i, 2)

	default:
		// Obfuscate nothing.
	}
	out.WriteString(strings.Join(args, " "))
}

// RemoveAllRedisArgs will take in a command and obfuscate all arguments following
// the command, regardless of if the command is valid Redis or not
func (*Obfuscator) RemoveAllRedisArgs(rediscmd string) string {
	fullCmd := strings.Fields(rediscmd)
	if len(fullCmd) == 0 {
		return ""
	}
	cmd, args := fullCmd[0], fullCmd[1:]

	var out strings.Builder
	out.WriteString(cmd)
	if len(args) == 0 {
		return out.String()
	}

	out.WriteByte(' ')
	switch strings.ToUpper(cmd) {
	case "BITFIELD":
		out.WriteString("?")
		for _, a := range args {
			arg := strings.ToUpper(a)
			if arg == "SET" || arg == "GET" || arg == "INCRBY" {
				out.WriteString(strings.Join([]string{"", a, "?"}, " "))
			}
		}
	case "CONFIG":
		arg := strings.ToUpper(args[0])
		if arg == "GET" || arg == "SET" || arg == "RESETSTAT" || arg == "REWRITE" {
			out.WriteString(strings.Join([]string{args[0], "?"}, " "))
		} else {
			out.WriteString("?")
		}
	default:
		out.WriteString("?")
	}

	return out.String()
}

func obfuscateRedisArgN(args []string, n int) {
	if len(args) > n {
		args[n] = "?"
	}
}

func obfuscateRedisArgsStep(args []string, start, step int) {
	if start+step-1 >= len(args) {
		// can't reach target
		return
	}
	for i := start + step - 1; i < len(args); i += step {
		args[i] = "?"
	}
}
