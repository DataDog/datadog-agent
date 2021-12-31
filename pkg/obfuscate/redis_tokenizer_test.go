// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package obfuscate

import (
	"strconv"
	"testing"
)

func TestRedisTokenizer(t *testing.T) {
	type testResult struct {
		tok  string
		typ  redisTokenType
		done bool
	}
	for ti, tt := range []struct {
		in  string
		out []testResult
	}{
		{
			in:  "",
			out: []testResult{{"", redisTokenCommand, true}},
		},
		{
			in: "BAD\"\"INPUT\" \"boo\n  Weird13\\Stuff",
			out: []testResult{
				{"BAD\"\"INPUT\"", redisTokenCommand, false},
				{"\"boo\n  Weird13\\Stuff", redisTokenArgument, true},
			},
		},
		{
			in: "CMD",
			out: []testResult{
				{"CMD", redisTokenCommand, true},
			},
		},
		{
			in: "\n  \nCMD\n  \n",
			out: []testResult{
				{"CMD", redisTokenCommand, true},
			},
		},
		{
			in: "  CMD  ",
			out: []testResult{
				{"CMD", redisTokenCommand, true},
			},
		},
		{
			in: "CMD1\nCMD2",
			out: []testResult{
				{"CMD1", redisTokenCommand, false},
				{"CMD2", redisTokenCommand, true},
			},
		},
		{
			in: "  CMD1  \n  CMD2  ",
			out: []testResult{
				{"CMD1", redisTokenCommand, false},
				{"CMD2", redisTokenCommand, true},
			},
		},
		{
			in: "CMD1\nCMD2\nCMD3",
			out: []testResult{
				{"CMD1", redisTokenCommand, false},
				{"CMD2", redisTokenCommand, false},
				{"CMD3", redisTokenCommand, true},
			},
		},
		{
			in: "CMD1 \n CMD2 \n CMD3 ",
			out: []testResult{
				{"CMD1", redisTokenCommand, false},
				{"CMD2", redisTokenCommand, false},
				{"CMD3", redisTokenCommand, true},
			},
		},
		{
			in: "CMD arg",
			out: []testResult{
				{"CMD", redisTokenCommand, false},
				{"arg", redisTokenArgument, true},
			},
		},
		{
			in: "  CMD  arg  ",
			out: []testResult{
				{"CMD", redisTokenCommand, false},
				{"arg", redisTokenArgument, true},
			},
		},
		{
			in: "CMD arg1 arg2",
			out: []testResult{
				{"CMD", redisTokenCommand, false},
				{"arg1", redisTokenArgument, false},
				{"arg2", redisTokenArgument, true},
			},
		},
		{
			in: " 	 CMD   arg1 	  arg2 ",
			out: []testResult{
				{"CMD", redisTokenCommand, false},
				{"arg1", redisTokenArgument, false},
				{"arg2", redisTokenArgument, true},
			},
		},
		{
			in: "CMD arg1\nCMD2 arg2",
			out: []testResult{
				{"CMD", redisTokenCommand, false},
				{"arg1", redisTokenArgument, false},
				{"CMD2", redisTokenCommand, false},
				{"arg2", redisTokenArgument, true},
			},
		},
		{
			in: "CMD arg1 arg2\nCMD2 arg3\nCMD3\nCMD4 arg4 arg5 arg6",
			out: []testResult{
				{"CMD", redisTokenCommand, false},
				{"arg1", redisTokenArgument, false},
				{"arg2", redisTokenArgument, false},
				{"CMD2", redisTokenCommand, false},
				{"arg3", redisTokenArgument, false},
				{"CMD3", redisTokenCommand, false},
				{"CMD4", redisTokenCommand, false},
				{"arg4", redisTokenArgument, false},
				{"arg5", redisTokenArgument, false},
				{"arg6", redisTokenArgument, true},
			},
		},
		{
			in: "CMD arg1   arg2  \n CMD2  arg3 \n CMD3 \n  CMD4 arg4 arg5 arg6\nCMD5 ",
			out: []testResult{
				{"CMD", redisTokenCommand, false},
				{"arg1", redisTokenArgument, false},
				{"arg2", redisTokenArgument, false},
				{"CMD2", redisTokenCommand, false},
				{"arg3", redisTokenArgument, false},
				{"CMD3", redisTokenCommand, false},
				{"CMD4", redisTokenCommand, false},
				{"arg4", redisTokenArgument, false},
				{"arg5", redisTokenArgument, false},
				{"arg6", redisTokenArgument, false},
				{"CMD5", redisTokenCommand, true},
			},
		},
		{
			in: `CMD ""`,
			out: []testResult{
				{"CMD", redisTokenCommand, false},
				{`""`, redisTokenArgument, true},
			},
		},
		{
			in: `CMD  "foo bar"`,
			out: []testResult{
				{"CMD", redisTokenCommand, false},
				{`"foo bar"`, redisTokenArgument, true},
			},
		},
		{
			in: `CMD  "foo bar\ " baz`,
			out: []testResult{
				{"CMD", redisTokenCommand, false},
				{`"foo bar\ "`, redisTokenArgument, false},
				{`baz`, redisTokenArgument, true},
			},
		},
		{
			in: "CMD \"foo \n bar\" \"\"  baz ",
			out: []testResult{
				{"CMD", redisTokenCommand, false},
				{"\"foo \n bar\"", redisTokenArgument, false},
				{`""`, redisTokenArgument, false},
				{"baz", redisTokenArgument, true},
			},
		},
		{
			in: "CMD \"foo \\\" bar\" baz",
			out: []testResult{
				{"CMD", redisTokenCommand, false},
				{"\"foo \\\" bar\"", redisTokenArgument, false},
				{"baz", redisTokenArgument, true},
			},
		},
		{
			in: `CMD  "foo bar"  baz`,
			out: []testResult{
				{"CMD", redisTokenCommand, false},
				{`"foo bar"`, redisTokenArgument, false},
				{`baz`, redisTokenArgument, true},
			},
		},
		{
			in: "CMD \"foo bar\" baz\nCMD2 \"baz\\\\bar\"",
			out: []testResult{
				{"CMD", redisTokenCommand, false},
				{`"foo bar"`, redisTokenArgument, false},
				{`baz`, redisTokenArgument, false},
				{"CMD2", redisTokenCommand, false},
				{`"baz\\bar"`, redisTokenArgument, true},
			},
		},
		{
			in: " CMD  \"foo bar\"  baz \n CMD2  \"baz\\\\bar\"  ",
			out: []testResult{
				{"CMD", redisTokenCommand, false},
				{`"foo bar"`, redisTokenArgument, false},
				{`baz`, redisTokenArgument, false},
				{"CMD2", redisTokenCommand, false},
				{`"baz\\bar"`, redisTokenArgument, true},
			},
		},
	} {
		t.Run(strconv.Itoa(ti), func(t *testing.T) {
			tokenizer := newRedisTokenizer([]byte(tt.in))
			for i := 0; i < len(tt.out); i++ {
				tok, typ, done := tokenizer.scan()
				if done != tt.out[i].done {
					t.Fatalf("%d: wanted done: %v, got: %v", i, tt.out[i].done, done)
				}
				if tok != tt.out[i].tok {
					t.Fatalf("%d: wanted token: %q, got: %q", i, tt.out[i].tok, tok)
				}
				if typ != tt.out[i].typ {
					t.Fatalf("%d: wanted type: %s, got: %s", i, tt.out[i].typ, typ)
				}
			}
		})
	}
}
