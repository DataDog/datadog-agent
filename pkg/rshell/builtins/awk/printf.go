// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package awk

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	MaxPrintfWidth     = 1 << 20
	MaxPrintfPrecision = 1 << 20
	MaxPrintfOutput    = 1 << 20
)

func formatPrintf(format string, args []value) (string, error) {
	return formatPrintfRuntime(nil, format, args)
}

func (rt *runtime) formatPrintf(format string, args []value) (string, error) {
	return formatPrintfRuntime(rt, format, args)
}

func formatPrintfRuntime(rt *runtime, format string, args []value) (string, error) {
	var b strings.Builder
	arg := 0
	for i := 0; i < len(format); i++ {
		if rt != nil {
			if err := rt.ctx.Err(); err != nil {
				return "", err
			}
		}
		if format[i] != '%' {
			if err := appendPrintfByte(&b, format[i]); err != nil {
				return "", err
			}
			continue
		}
		start := i
		i++
		if i >= len(format) {
			if err := appendPrintfString(&b, format[start:]); err != nil {
				return "", err
			}
			return b.String(), nil
		}
		if format[i] == '%' {
			if err := appendPrintfByte(&b, '%'); err != nil {
				return "", err
			}
			continue
		}
		flagsStart := i
		for i < len(format) && strings.ContainsRune("-+ #0", rune(format[i])) {
			i++
		}
		flags := format[flagsStart:i]
		width := ""
		if i < len(format) && format[i] == '*' {
			v, err := nextPrintfArg(args, &arg)
			if err != nil {
				return "", err
			}
			dynamicWidth, err := dynamicPrintfBound(v, MaxPrintfWidth, "width", false)
			if err != nil {
				return "", err
			}
			if dynamicWidth < 0 {
				dynamicWidth = -dynamicWidth
				flags = strings.ReplaceAll(flags, "0", "")
				if !strings.ContainsRune(flags, '-') {
					flags += "-"
				}
			}
			width = strconv.Itoa(dynamicWidth)
			i++
		} else {
			widthStart := i
			if err := consumePrintfBound(format, &i, MaxPrintfWidth, "width"); err != nil {
				return "", err
			}
			width = format[widthStart:i]
		}
		precision := ""
		if i < len(format) && format[i] == '.' {
			i++
			precisionStart := i
			if i < len(format) && format[i] == '*' {
				v, err := nextPrintfArg(args, &arg)
				if err != nil {
					return "", err
				}
				n, err := dynamicPrintfBound(v, MaxPrintfPrecision, "precision", true)
				if err != nil {
					return "", err
				}
				i++
				if n >= 0 {
					precision = "." + strconv.Itoa(n)
				}
			} else {
				if err := consumePrintfBound(format, &i, MaxPrintfPrecision, "precision"); err != nil {
					return "", err
				}
				precision = "." + format[precisionStart:i]
			}
		}
		if i >= len(format) {
			return "", fmt.Errorf("unterminated printf format")
		}
		verb := format[i]
		if precision == "" && (verb == 'g' || verb == 'G') {
			precision = ".6"
		}
		v, err := nextPrintfArg(args, &arg)
		if err != nil {
			return "", err
		}
		spec := "%" + flags + width + precision + string(verb)
		flagsEnd := 1 + len(flags)
		var out string
		switch {
		case verb == 's':
			s, err := printfString(rt, v)
			if err != nil {
				return "", err
			}
			out = fmt.Sprintf(stripPrintfZeroFlag(spec, flagsEnd), s)
		case verb == 'd' || verb == 'i':
			out = fmt.Sprintf(withPrintfVerb(spec, 'd'), int64(v.Number()))
		case verb == 'u':
			out = fmt.Sprintf(normalizePrintfUnsigned(spec), uint64(v.Number()))
		case strings.ContainsRune("oxX", rune(verb)):
			n := uint64(v.Number())
			if n == 0 && (verb == 'x' || verb == 'X') {
				spec = strings.ReplaceAll(spec[:flagsEnd], "#", "") + spec[flagsEnd:]
			}
			out = fmt.Sprintf(spec, n)
		case strings.ContainsRune("eEfFgG", rune(verb)):
			out = fmt.Sprintf(spec, v.Number())
		case verb == 'c':
			out = fmt.Sprintf(stripPrintfZeroFlag(spec, flagsEnd), printfRune(v))
		default:
			return "", fmt.Errorf("unsupported printf format %%%c", verb)
		}
		if err := appendPrintfString(&b, out); err != nil {
			return "", err
		}
	}
	return b.String(), nil
}

func nextPrintfArg(args []value, arg *int) (value, error) {
	if *arg >= len(args) {
		return value{}, fmt.Errorf("fatal: not enough arguments for printf")
	}
	v := args[*arg]
	(*arg)++
	return v, nil
}

func dynamicPrintfBound(v value, max int, name string, negativeOmitted bool) (int, error) {
	n := math.Trunc(v.Number())
	if math.IsNaN(n) {
		return 0, nil
	}
	if math.IsInf(n, 0) {
		return 0, fmt.Errorf("printf %s exceeds %d", name, max)
	}
	if negativeOmitted && n < 0 {
		return -1, nil
	}
	if n > float64(max) || n < -float64(max) {
		return 0, fmt.Errorf("printf %s exceeds %d", name, max)
	}
	return int(n), nil
}

func printfString(rt *runtime, v value) (string, error) {
	if rt == nil {
		return v.String(), nil
	}
	return rt.conversionString(v, "CONVFMT")
}

func printfRune(v value) rune {
	if v.kind == valueString && v.s != "" {
		r, _ := utf8.DecodeRuneInString(v.s)
		return r
	}
	return rune(int64(v.Number()))
}

func withPrintfVerb(spec string, verb byte) string {
	return spec[:len(spec)-1] + string(verb)
}

func normalizePrintfUnsigned(spec string) string {
	prefix := spec[:len(spec)-1]
	prefix = strings.ReplaceAll(prefix, "+", "")
	prefix = strings.ReplaceAll(prefix, " ", "")
	prefix = strings.ReplaceAll(prefix, "#", "")
	return prefix + "d"
}

func stripPrintfZeroFlag(spec string, flagsEnd int) string {
	flags := strings.ReplaceAll(spec[:flagsEnd], "0", "")
	return flags + spec[flagsEnd:]
}

func appendPrintfByte(b *strings.Builder, c byte) error {
	if b.Len() >= MaxPrintfOutput {
		return fmt.Errorf("printf output exceeds %d bytes", MaxPrintfOutput)
	}
	b.WriteByte(c)
	return nil
}

func appendPrintfString(b *strings.Builder, s string) error {
	if len(s) > MaxPrintfOutput-b.Len() {
		return fmt.Errorf("printf output exceeds %d bytes", MaxPrintfOutput)
	}
	b.WriteString(s)
	return nil
}

func consumePrintfBound(format string, idx *int, max int, name string) error {
	n := 0
	for *idx < len(format) && format[*idx] >= '0' && format[*idx] <= '9' {
		digit := int(format[*idx] - '0')
		if n > (max-digit)/10 {
			return fmt.Errorf("printf %s exceeds %d", name, max)
		}
		n = n*10 + digit
		(*idx)++
	}
	if n > max {
		return fmt.Errorf("printf %s exceeds %d", name, max)
	}
	return nil
}
