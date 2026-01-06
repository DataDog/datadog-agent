// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"bytes"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"strings"
	"unicode"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// LogTimeSeriesAnalysis converts logs into timeseries metric outputs:
// - JSON logs: numeric fields -> Avg aggregation
// - Unstructured logs: pattern frequency -> Sum aggregation
//
// This is intentionally minimal; cardinality controls live in the observer storage (Step 5).
type LogTimeSeriesAnalysis struct {
	// MaxEvalBytes caps how many bytes we evaluate for unstructured signature generation (0 = no cap).
	MaxEvalBytes int

	// IncludeFields, if non-empty, restricts JSON numeric extraction to these field names.
	IncludeFields map[string]struct{}
	// ExcludeFields always excludes JSON fields from numeric extraction.
	ExcludeFields map[string]struct{}
}

func (a *LogTimeSeriesAnalysis) Name() string { return "log_timeseries" }

func (a *LogTimeSeriesAnalysis) Process(log observer.LogView) observer.LogProcessorResult {
	content := log.GetContent()
	tags := log.GetTags()

	// Always emit pattern frequency metric for all logs
	patternSig := logSignature(content, a.MaxEvalBytes)
	if patternSig == "" {
		return observer.LogProcessorResult{}
	}

	metrics := []observer.MetricOutput{{
		Name:  patternCountMetricName(patternSig),
		Value: 1,
		Tags:  tags,
	}}

	// For JSON logs, also extract numeric field metrics
	if isJSONObject(content) {
		metrics = append(metrics, a.extractJSONFieldMetrics(content, tags)...)
	}

	return observer.LogProcessorResult{Metrics: metrics}
}

func isJSONObject(b []byte) bool {
	trimmed := bytes.TrimSpace(b)
	return len(trimmed) > 1 && trimmed[0] == '{' && json.Valid(trimmed)
}

// extractJSONFieldMetrics extracts numeric field metrics from JSON content.
// Pattern metrics are handled separately in Analyze().
func (a *LogTimeSeriesAnalysis) extractJSONFieldMetrics(content []byte, tags []string) []observer.MetricOutput {
	dec := json.NewDecoder(bytes.NewReader(content))
	dec.UseNumber()

	var obj map[string]any
	if err := dec.Decode(&obj); err != nil {
		return nil
	}

	var out []observer.MetricOutput
	for k, v := range obj {
		if a.ExcludeFields != nil {
			if _, ok := a.ExcludeFields[k]; ok {
				continue
			}
		}
		if len(a.IncludeFields) > 0 {
			if _, ok := a.IncludeFields[k]; !ok {
				continue
			}
		}

		f, ok := coerceNumber(v)
		if !ok {
			continue
		}

		out = append(out, observer.MetricOutput{
			Name:  "log.field." + sanitizeMetricFragment(k),
			Value: f,
			Tags:  tags,
		})
	}

	return out
}

func coerceNumber(v any) (float64, bool) {
	switch n := v.(type) {
	case json.Number:
		// Prefer float to support decimals; json.Number will parse ints as well.
		f, err := n.Float64()
		return f, err == nil
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case int32:
		return float64(n), true
	case uint64:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint:
		return float64(n), true
	default:
		return 0, false
	}
}

func patternCountMetricName(signature string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(signature))
	return fmt.Sprintf("log.pattern.%x.count", h.Sum64())
}

func sanitizeMetricFragment(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "unknown"
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(unicode.ToLower(r))
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '_' || r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}

// ---------------------------------------------------------------------------
// Inlined log signature generation (avoids dependency on pkg/logs/pattern)
// ---------------------------------------------------------------------------

// maxRun is the maximum run of a char or digit before it is capped.
const maxRun = 10

// Token types for log signature generation
type sigToken byte

const (
	tokSpace sigToken = iota
	tokColon
	tokSemicolon
	tokDash
	tokUnderscore
	tokFslash
	tokBslash
	tokPeriod
	tokComma
	tokSinglequote
	tokDoublequote
	tokBacktick
	tokTilda
	tokStar
	tokPlus
	tokEqual
	tokParenopen
	tokParenclose
	tokBraceopen
	tokBraceclose
	tokBracketopen
	tokBracketclose
	tokAmpersand
	tokExclamation
	tokAt
	tokPound
	tokDollar
	tokPercent
	tokUparrow
	// Digit runs D1..D10
	tokD1
	tokD2
	tokD3
	tokD4
	tokD5
	tokD6
	tokD7
	tokD8
	tokD9
	tokD10
	// Char runs C1..C10
	tokC1
	tokC2
	tokC3
	tokC4
	tokC5
	tokC6
	tokC7
	tokC8
	tokC9
	tokC10
	// Special tokens
	tokMonth
	tokDay
	tokApm
	tokZone
	tokT
	tokEnd
)

// logSignature returns a deterministic signature for the input bytes, capped to maxEvalBytes if > 0.
func logSignature(input []byte, maxEvalBytes int) string {
	if len(input) == 0 {
		return ""
	}
	maxBytes := len(input)
	if maxEvalBytes > 0 && maxBytes > maxEvalBytes {
		maxBytes = maxEvalBytes
	}
	input = input[:maxBytes]

	var out strings.Builder
	out.Grow(len(input))

	var buf [maxRun]byte
	bufLen := 0
	resetBuf := func() { bufLen = 0 }
	appendBuf := func(b byte) {
		if bufLen < len(buf) {
			if b >= 'a' && b <= 'z' {
				b = b - 'a' + 'A'
			}
			buf[bufLen] = b
			bufLen++
		}
	}

	run := 0
	lastToken := getTokenType(input[0])
	resetBuf()
	appendBuf(input[0])

	insertToken := func() {
		defer func() {
			run = 0
			resetBuf()
		}()

		if lastToken == tokC1 {
			if bufLen == 1 {
				if specialToken := getSpecialShortToken(buf[0]); specialToken != tokEnd {
					out.WriteString(sigTokenToString(specialToken))
					return
				}
			} else if bufLen > 1 {
				if specialToken := getSpecialLongToken(string(buf[:bufLen])); specialToken != tokEnd {
					out.WriteString(sigTokenToString(specialToken))
					return
				}
			}
		}

		if lastToken == tokC1 || lastToken == tokD1 {
			if run >= maxRun {
				run = maxRun - 1
			}
			out.WriteString(sigTokenToString(lastToken + sigToken(run)))
			return
		}

		out.WriteString(sigTokenToString(lastToken))
	}

	for _, char := range input[1:] {
		currentToken := getTokenType(char)
		if currentToken != lastToken {
			insertToken()
		} else {
			run++
		}
		appendBuf(char)
		lastToken = currentToken
	}
	insertToken()

	return out.String()
}

func getTokenType(char byte) sigToken {
	if unicode.IsDigit(rune(char)) {
		return tokD1
	} else if unicode.IsSpace(rune(char)) {
		return tokSpace
	}

	switch char {
	case ':':
		return tokColon
	case ';':
		return tokSemicolon
	case '-':
		return tokDash
	case '_':
		return tokUnderscore
	case '/':
		return tokFslash
	case '\\':
		return tokBslash
	case '.':
		return tokPeriod
	case ',':
		return tokComma
	case '\'':
		return tokSinglequote
	case '"':
		return tokDoublequote
	case '`':
		return tokBacktick
	case '~':
		return tokTilda
	case '*':
		return tokStar
	case '+':
		return tokPlus
	case '=':
		return tokEqual
	case '(':
		return tokParenopen
	case ')':
		return tokParenclose
	case '{':
		return tokBraceopen
	case '}':
		return tokBraceclose
	case '[':
		return tokBracketopen
	case ']':
		return tokBracketclose
	case '&':
		return tokAmpersand
	case '!':
		return tokExclamation
	case '@':
		return tokAt
	case '#':
		return tokPound
	case '$':
		return tokDollar
	case '%':
		return tokPercent
	case '^':
		return tokUparrow
	}

	return tokC1
}

func getSpecialShortToken(char byte) sigToken {
	switch char {
	case 'T':
		return tokT
	case 'Z':
		return tokZone
	}
	return tokEnd
}

func getSpecialLongToken(input string) sigToken {
	switch input {
	case "JAN", "FEB", "MAR", "APR", "MAY", "JUN", "JUL",
		"AUG", "SEP", "OCT", "NOV", "DEC":
		return tokMonth
	case "MON", "TUE", "WED", "THU", "FRI", "SAT", "SUN":
		return tokDay
	case "AM", "PM":
		return tokApm
	case "UTC", "GMT", "EST", "EDT", "CST", "CDT",
		"MST", "MDT", "PST", "PDT", "JST", "KST",
		"IST", "MSK", "CEST", "CET", "BST", "NZST",
		"NZDT", "ACST", "ACDT", "AEST", "AEDT",
		"AWST", "AWDT", "AKST", "AKDT", "HST",
		"HDT", "CHST", "CHDT", "NST", "NDT":
		return tokZone
	}
	return tokEnd
}

func sigTokenToString(token sigToken) string {
	if token >= tokD1 && token <= tokD10 {
		return strings.Repeat("D", int(token-tokD1)+1)
	} else if token >= tokC1 && token <= tokC10 {
		return strings.Repeat("C", int(token-tokC1)+1)
	}

	switch token {
	case tokSpace:
		return " "
	case tokColon:
		return ":"
	case tokSemicolon:
		return ";"
	case tokDash:
		return "-"
	case tokUnderscore:
		return "_"
	case tokFslash:
		return "/"
	case tokBslash:
		return "\\"
	case tokPeriod:
		return "."
	case tokComma:
		return ","
	case tokSinglequote:
		return "'"
	case tokDoublequote:
		return "\""
	case tokBacktick:
		return "`"
	case tokTilda:
		return "~"
	case tokStar:
		return "*"
	case tokPlus:
		return "+"
	case tokEqual:
		return "="
	case tokParenopen:
		return "("
	case tokParenclose:
		return ")"
	case tokBraceopen:
		return "{"
	case tokBraceclose:
		return "}"
	case tokBracketopen:
		return "["
	case tokBracketclose:
		return "]"
	case tokAmpersand:
		return "&"
	case tokExclamation:
		return "!"
	case tokAt:
		return "@"
	case tokPound:
		return "#"
	case tokDollar:
		return "$"
	case tokPercent:
		return "%"
	case tokUparrow:
		return "^"
	case tokMonth:
		return "MTH"
	case tokDay:
		return "DAY"
	case tokApm:
		return "PM"
	case tokT:
		return "T"
	case tokZone:
		return "ZONE"
	}
	return ""
}
