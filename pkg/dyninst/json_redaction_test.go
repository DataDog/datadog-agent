// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package dyninst_test

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/go-json-experiment/json/jsontext"
	"github.com/stretchr/testify/require"
)

type matcher interface {
	matches(ptr jsontext.Pointer) bool
}

type replacer interface {
	replace(jsontext.Value) jsontext.Value
}

type regexpReplacer regexp.Regexp

func newRegexpReplacer(re string) *regexpReplacer {
	return (*regexpReplacer)(regexp.MustCompile(re))
}

func (r *regexpReplacer) replace(v jsontext.Value) jsontext.Value {
	re := (*regexp.Regexp)(r)
	if v.Kind() != '"' {
		return v
	}
	var s string
	_ = json.Unmarshal(v, &s)
	match := re.FindStringSubmatchIndex(s)
	if len(match) == 0 {
		return v
	}
	var offset int
	var sb strings.Builder
	names := re.SubexpNames()
	for i := 2; i < len(match); i += 2 {
		if match[i] < 0 {
			return jsontext.Value(`"invalid match: overlaps"`)
		}
		sb.WriteString(s[offset:match[i]])
		if name := names[i/2]; name != "" {
			sb.WriteRune('[')
			sb.WriteString(name)
			sb.WriteRune(']')
		} else {
			sb.WriteString(s[match[i]:match[i+1]])
		}
		offset = match[i+1]
	}
	sb.WriteString(s[offset:])
	marshalled, _ := json.Marshal(sb.String())
	return jsontext.Value(marshalled)
}

type jsonRedactor struct {
	matcher  matcher
	replacer replacer
}

type exactMatcher string

func (m exactMatcher) matches(ptr jsontext.Pointer) bool {
	return string(ptr) == string(m)
}

type reMatcher regexp.Regexp

func matchRegexp(re string) matcher {
	return (*reMatcher)(regexp.MustCompile(re))
}

func (m *reMatcher) matches(ptr jsontext.Pointer) bool {
	return (*regexp.Regexp)(m).MatchString(string(ptr))
}

type replacement jsontext.Value

func (r replacement) replace(jsontext.Value) jsontext.Value {
	return jsontext.Value(r)
}

type prefixSuffixMatcher [2]string

func prefixMatcher(prefix string) prefixSuffixMatcher {
	return prefixSuffixMatcher{prefix, ""}
}

func (m prefixSuffixMatcher) matches(ptr jsontext.Pointer) bool {
	return strings.HasPrefix(string(ptr), m[0]) &&
		strings.HasSuffix(string(ptr), m[1])
}

type replacerFunc func(jsontext.Value) jsontext.Value

func (f replacerFunc) replace(v jsontext.Value) jsontext.Value {
	return f(v)
}

func redactor(matcher matcher, replacer replacer) jsonRedactor {
	return jsonRedactor{
		matcher:  matcher,
		replacer: replacer,
	}
}

type entriesSorter struct{}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

var notCapturedReason = []byte(`"notCapturedReason"`)

func compareNotCapturedReason(a, b []byte) int {
	return cmp.Compare(
		boolToInt(bytes.Contains(a, notCapturedReason)),
		boolToInt(bytes.Contains(b, notCapturedReason)),
	)
}

func (e entriesSorter) replace(v jsontext.Value) jsontext.Value {
	var entries [][2]jsontext.Value
	if err := json.Unmarshal(v, &entries); err != nil {
		return v // Return original value if unmarshal fails
	}
	slices.SortFunc(entries, func(a, b [2]jsontext.Value) int {
		return cmp.Or(
			compareNotCapturedReason(a[0], b[0]),
			bytes.Compare(a[0], b[0]),
		)
	})
	sorted, err := json.Marshal(entries)
	if err != nil {
		return v // Return original value if marshal fails
	}
	return sorted
}

type stackFrame struct {
	Function   string `json:"function"`
	FileName   string `json:"fileName"`
	LineNumber any    `json:"lineNumber"`
}

func redactStackFrame(v jsontext.Value) jsontext.Value {
	if v.Kind() != '{' {
		return v
	}
	d := json.NewDecoder(bytes.NewReader(v))
	d.UseNumber()
	var f stackFrame
	err := d.Decode(&f)
	if err != nil {
		return v
	}
	isAsm := strings.HasPrefix(f.FileName, "runtime/asm_")
	isRuntime := strings.HasPrefix(f.FileName, "runtime/")
	if !isAsm && !isRuntime {
		return v
	}
	if isAsm {
		f.FileName = "runtime/asm_[arch].s"
	}
	f.LineNumber = "[lineNumber]"
	buf, err := json.Marshal(f)
	if err != nil {
		return v
	}
	return jsontext.Value(buf)
}

var defaultRedactors = []jsonRedactor{
	redactor(
		exactMatcher(`/logger/thread_id`),
		replacerFunc(redactGoID),
	),
	redactor(
		matchRegexp(`/debugger/snapshot/stack/[[:digit:]]+$`),
		replacerFunc(redactStackFrame),
	),
	redactor(
		exactMatcher(`/debugger/snapshot/id`),
		replacement(`"[id]"`),
	),
	redactor(
		exactMatcher(`/debugger/snapshot/timestamp`),
		replacement(`"[ts]"`),
	),
	redactor(
		exactMatcher(`/timestamp`),
		replacement(`"[ts]"`),
	),
	redactor(
		exactMatcher(`/duration`),
		replacerFunc(redactNonZeroDuration),
	),
	redactor(
		prefixSuffixMatcher{"/debugger/snapshot/captures/", "/address"},
		replacerFunc(redactNonZeroAddress),
	),
	redactor(
		exactMatcher(`/debugger/snapshot/captures/return/locals/~0r0/value`),
		regexpStringReplacer("0x[[:xdigit:]]+", "0x[addr]"),
	),
	redactor(
		prefixSuffixMatcher{"/debugger/snapshot/captures/entry/arguments/redactMyEntries", "/entries"},
		replacement(`"[redacted-entries]"`),
	),
	redactor(
		prefixSuffixMatcher{"/debugger/snapshot/captures/", "/entries"},
		entriesSorter{},
	),
	redactor(
		matchRegexp(`/debugger/snapshot/captures/entry/arguments/.*`),
		replacerFunc(redactTypesThatDependOnVersion),
	),
	redactor(
		matchRegexp(`/debugger/snapshot/captures/entry/arguments/.*/type`),
		regexpStringReplacer(
			`UnknownType\(0x[[:xdigit:]]+\)`,
			`UnknownType(0x[GoRuntimeType])`,
		),
	),
	redactor(
		exactMatcher(`/message`),
		regexpStringReplacer(
			`0x[[:xdigit:]]+`,
			`0x[addr]`,
		),
	),
	redactor(
		exactMatcher(`/message`),
		regexpStringReplacer(
			`map\[.*, \.\.\.\]`,
			`map[{redacted-entries}, ...]`,
		),
	),
	redactor(
		exactMatcher(`/message`),
		regexpStringReplacer(
			`\{mu: `+mutexInternalsRegexp+`\}|`+mutexInternalsRegexp,
			`{mutex internals}`,
		),
	),
	redactor(
		exactMatcher(`/message`),
		regexpStringReplacer(
			`[0-9]+\.[0-9]+ms`,
			`[duration]ms`,
		),
	),
}

const mutexInternalsRegexp = `\{state: [[:digit:]]+, sema: [[:digit:]]+\}`

func redactGoID(v jsontext.Value) jsontext.Value {
	if v.Kind() != '0' {
		return v
	}
	var goid uint64
	if err := json.Unmarshal(v, &goid); err != nil {
		return v
	}
	if goid == 0 {
		return v
	}
	buf, err := json.Marshal("[goid]")
	if err != nil {
		return v
	}
	return jsontext.Value(buf)
}

func redactNonZeroDuration(v jsontext.Value) jsontext.Value {
	if v.Kind() != '0' {
		return v
	}
	var duration int64
	if err := json.Unmarshal(v, &duration); err != nil {
		return v
	}
	if duration == 0 {
		return v
	}
	buf, err := json.Marshal("[duration]")
	if err != nil {
		return v
	}
	return jsontext.Value(buf)
}

func redactNonZeroAddress(v jsontext.Value) jsontext.Value {
	if v.Kind() != '"' {
		return v
	}
	var s string
	if err := json.Unmarshal(v, &s); err != nil {
		return v
	}
	addr, err := strconv.ParseUint(s, 0, 64)
	if err != nil {
		return v
	}
	if addr == 0 {
		return v
	}
	s = "[addr]"
	buf, err := json.Marshal(s)
	if err != nil {
		return v
	}
	return jsontext.Value(buf)
}

// The structure of some types from the stdlib changed over time. Redact them so
// that golden files are valid across versions.
func redactTypesThatDependOnVersion(v jsontext.Value) jsontext.Value {
	var t = struct {
		Type string `json:"type"`
	}{}
	if err := json.Unmarshal(v, &t); err != nil {
		return v
	}
	if t.Type != "sync.Mutex" && t.Type != "sync.Once" {
		return v
	}
	return jsontext.Value(fmt.Sprintf(
		`"[%s (different in different versions)]"`,
		t.Type))
}

func regexpStringReplacer(pat, replacement string) replacer {
	re := regexp.MustCompile(pat)
	return replacerFunc(func(v jsontext.Value) jsontext.Value {
		var s string
		if err := json.Unmarshal(v, &s); err != nil {
			return v
		}
		if !re.MatchString(s) {
			return v
		}
		replaced := re.ReplaceAllString(s, replacement)
		marshalled, err := json.Marshal(replaced)
		if err != nil {
			return v
		}
		return jsontext.Value(marshalled)
	})
}

func redactJSON(t *testing.T, ptrPrefix jsontext.Pointer, input []byte, redactors []jsonRedactor) []byte {
	d := jsontext.NewDecoder(bytes.NewReader(input))
	var buf bytes.Buffer
	e := jsontext.NewEncoder(
		&buf,
		jsontext.WithIndent("  "),
		jsontext.WithIndentPrefix("  "),
		jsontext.PreserveRawStrings(false),
		jsontext.EscapeForHTML(false),
		jsontext.EscapeForJS(false),
	)
	ptrPrefix = jsontext.Pointer(strings.TrimSuffix(string(ptrPrefix), "/"))
	stackPtr := func() jsontext.Pointer {
		return jsontext.Pointer(
			string(ptrPrefix) + "/" +
				strings.TrimPrefix(string(d.StackPointer()), "/"))
	}
	copyToken := func() (done bool) {
		tok, err := d.ReadToken()
		if errors.Is(err, io.EOF) {
			return true
		}
		err = e.WriteToken(tok)
		require.NoError(t, err)
		return false
	}
	for {
		kind, idx := d.StackIndex(d.StackDepth())
		if kind == 0 {
			if copyToken() {
				break
			}
			continue
		}
		// If we're in an object and this is a key, copy it across.
		switch d.PeekKind() {
		case ']', '}':
			require.False(t, copyToken(), "unexpected EOF")
			continue
		}
		if kind == '{' && idx%2 == 0 {
			require.False(t, copyToken(), "unexpected EOF")
		}

		ptr := stackPtr()
		var redacted []byte
		for _, redactor := range redactors {
			if redactor.matcher.matches(ptr) {
				if redacted == nil {
					v, err := d.ReadValue()
					require.NoError(t, err)
					redacted = v
				}
				redacted = redactor.replacer.replace(redacted)
			}
		}

		// If we read a whole value, recursively redact it and write that out,
		// otherwise just copy the token across.
		if redacted != nil {
			switch jsontext.Value(redacted).Kind() {
			case '{', '[': // apply recursive redaction
				redacted = redactJSON(t, ptr, redacted, redactors)
			}
			require.NoError(t, e.WriteValue(redacted))
		} else if copyToken() {
			break
		}
	}
	return bytes.TrimSpace(buf.Bytes())
}
