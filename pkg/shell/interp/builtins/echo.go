// Copyright (c) Datadog, Inc.
// See LICENSE for licensing information

package builtins

import (
	"context"
	"strings"
)

func builtinEcho(_ context.Context, callCtx *CallContext, args []string) Result {
	noNewline := false
	interpretEscapes := false

	i := 0
	for i < len(args) {
		arg := args[i]
		if len(arg) < 2 || arg[0] != '-' {
			break
		}
		valid := true
		for _, c := range arg[1:] {
			if c != 'n' && c != 'e' && c != 'E' {
				valid = false
				break
			}
		}
		if !valid {
			break
		}
		for _, c := range arg[1:] {
			switch c {
			case 'n':
				noNewline = true
			case 'e':
				interpretEscapes = true
			case 'E':
				interpretEscapes = false
			}
		}
		i++
	}
	args = args[i:]

	for j, arg := range args {
		if j > 0 {
			callCtx.Out(" ")
		}
		if interpretEscapes {
			if !echoEscape(callCtx, arg) {
				return Result{}
			}
		} else {
			callCtx.Out(arg)
		}
	}
	if !noNewline {
		callCtx.Out("\n")
	}
	return Result{}
}

func echoEscape(callCtx *CallContext, s string) bool {
	var buf strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 >= len(s) {
			buf.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case 'a':
			buf.WriteByte('\a')
		case 'b':
			buf.WriteByte('\b')
		case 'c':
			callCtx.Out(buf.String())
			return false
		case 'e', 'E':
			buf.WriteByte(0x1b)
		case 'f':
			buf.WriteByte('\f')
		case 'n':
			buf.WriteByte('\n')
		case 'r':
			buf.WriteByte('\r')
		case 't':
			buf.WriteByte('\t')
		case 'v':
			buf.WriteByte('\v')
		case '\\':
			buf.WriteByte('\\')
		case '0':
			val := 0
			j := 0
			for j < 3 && i+1+j < len(s) && s[i+1+j] >= '0' && s[i+1+j] <= '7' {
				val = val*8 + int(s[i+1+j]-'0')
				j++
			}
			i += j
			buf.WriteByte(byte(val))
		case 'x':
			val, j := 0, 0
			for j < 2 && i+1+j < len(s) {
				c := s[i+1+j]
				switch {
				case c >= '0' && c <= '9':
					val = val*16 + int(c-'0')
				case c >= 'a' && c <= 'f':
					val = val*16 + int(c-'a'+10)
				case c >= 'A' && c <= 'F':
					val = val*16 + int(c-'A'+10)
				default:
					goto doneHex
				}
				j++
			}
		doneHex:
			if j == 0 {
				buf.WriteByte('\\')
				buf.WriteByte('x')
			} else {
				i += j
				buf.WriteByte(byte(val))
			}
		default:
			buf.WriteByte('\\')
			buf.WriteByte(s[i])
		}
	}
	callCtx.Out(buf.String())
	return true
}
