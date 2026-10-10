// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package store

import (
	"fmt"
	"path"
	"strconv"
	"strings"
	"unicode"
)

// Event is the view of a log or span that search queries are evaluated against.
type Event interface {
	// Field returns the values of a reserved field (service, host, status...) or attribute (@foo.bar).
	Field(name string) []string
	// Text returns the full-text searchable content.
	Text() string
}

// Query is a parsed Datadog search query, e.g. `service:web -status:info @http.status_code:>=500 "timeout"`.
type Query struct {
	root node
}

type node interface {
	match(Event) bool
}

type andNode []node
type orNode []node
type notNode struct{ n node }
type textNode struct{ text string }
type fieldNode struct {
	field string
	value string
	op    string // "", ">", ">=", "<", "<=", "range"
	lo    string
	hi    string
}
type matchAll struct{}

func (a andNode) match(e Event) bool {
	for _, n := range a {
		if !n.match(e) {
			return false
		}
	}
	return true
}

func (o orNode) match(e Event) bool {
	for _, n := range o {
		if n.match(e) {
			return true
		}
	}
	return false
}

func (n notNode) match(e Event) bool { return !n.n.match(e) }
func (matchAll) match(Event) bool    { return true }

func (t textNode) match(e Event) bool {
	return globContains(strings.ToLower(e.Text()), strings.ToLower(t.text))
}

func globContains(haystack, needle string) bool {
	if needle == "*" || needle == "" {
		return true
	}
	if strings.ContainsAny(needle, "*?") {
		ok, _ := path.Match("*"+needle+"*", haystack)
		return ok
	}
	return strings.Contains(haystack, needle)
}

func (f fieldNode) match(e Event) bool {
	values := e.Field(f.field)
	if f.value == "*" && f.op == "" {
		return len(values) > 0
	}
	for _, v := range values {
		if f.matchValue(v) {
			return true
		}
	}
	return false
}

func compareNum(v string, bound string) (int, bool) {
	a, err1 := strconv.ParseFloat(v, 64)
	b, err2 := strconv.ParseFloat(bound, 64)
	if err1 != nil || err2 != nil {
		return strings.Compare(v, bound), true
	}
	switch {
	case a < b:
		return -1, true
	case a > b:
		return 1, true
	}
	return 0, true
}

func (f fieldNode) matchValue(v string) bool {
	switch f.op {
	case ">", ">=", "<", "<=":
		c, ok := compareNum(v, f.value)
		if !ok {
			return false
		}
		switch f.op {
		case ">":
			return c > 0
		case ">=":
			return c >= 0
		case "<":
			return c < 0
		default:
			return c <= 0
		}
	case "range":
		lo, ok1 := compareNum(v, f.lo)
		hi, ok2 := compareNum(v, f.hi)
		return ok1 && ok2 && (f.lo == "*" || lo >= 0) && (f.hi == "*" || hi <= 0)
	}
	if strings.ContainsAny(f.value, "*?") {
		ok, _ := path.Match(strings.ToLower(f.value), strings.ToLower(v))
		return ok
	}
	return strings.EqualFold(v, f.value)
}

// Match reports whether the event satisfies the query.
func (q *Query) Match(e Event) bool {
	if q == nil || q.root == nil {
		return true
	}
	return q.root.match(e)
}

// ParseQuery parses a Datadog search query. Unparseable fragments degrade to full-text terms
// rather than errors so the UI never breaks on partial input.
func ParseQuery(s string) *Query {
	p := &parser{toks: tokenize(s)}
	n := p.parseOr()
	if n == nil {
		n = matchAll{}
	}
	return &Query{root: n}
}

type token struct {
	kind string // "word", "quoted", "(", ")", "OR", "AND", "NOT", "-"
	val  string
}

func tokenize(s string) []token {
	var toks []token
	rs := []rune(s)
	i := 0
	for i < len(rs) {
		r := rs[i]
		switch {
		case unicode.IsSpace(r):
			i++
		case r == '(' || r == ')':
			toks = append(toks, token{kind: string(r)})
			i++
		case r == '-' && (i+1 < len(rs) && !unicode.IsSpace(rs[i+1])):
			toks = append(toks, token{kind: "-"})
			i++
		case r == '"':
			j := i + 1
			var sb strings.Builder
			for j < len(rs) && rs[j] != '"' {
				if rs[j] == '\\' && j+1 < len(rs) {
					j++
				}
				sb.WriteRune(rs[j])
				j++
			}
			toks = append(toks, token{kind: "quoted", val: sb.String()})
			i = j + 1
		default:
			j := i
			var sb strings.Builder
			depth := 0
			for j < len(rs) {
				c := rs[j]
				if c == '\\' && j+1 < len(rs) {
					sb.WriteRune(rs[j+1])
					j += 2
					continue
				}
				if c == '[' {
					depth++
				}
				if c == ']' {
					depth--
				}
				if c == '(' && strings.HasSuffix(sb.String(), ":") {
					// field:(a OR b): keep the group in the term
					k := j
					for k < len(rs) && rs[k] != ')' {
						sb.WriteRune(rs[k])
						k++
					}
					sb.WriteRune(')')
					j = k + 1
					continue
				}
				if depth <= 0 && (unicode.IsSpace(c) || c == '(' || c == ')') {
					break
				}
				if c == '"' && strings.HasSuffix(sb.String(), ":") {
					// field:"quoted value"
					k := j + 1
					for k < len(rs) && rs[k] != '"' {
						sb.WriteRune(rs[k])
						k++
					}
					j = k + 1
					continue
				}
				sb.WriteRune(c)
				j++
			}
			w := sb.String()
			switch w {
			case "OR", "||":
				toks = append(toks, token{kind: "OR"})
			case "AND", "&&":
				toks = append(toks, token{kind: "AND"})
			case "NOT":
				toks = append(toks, token{kind: "NOT"})
			default:
				toks = append(toks, token{kind: "word", val: w})
			}
			i = j
		}
	}
	return toks
}

type parser struct {
	toks []token
	pos  int
}

func (p *parser) peek() *token {
	if p.pos >= len(p.toks) {
		return nil
	}
	return &p.toks[p.pos]
}

func (p *parser) parseOr() node {
	var parts []node
	if n := p.parseAnd(); n != nil {
		parts = append(parts, n)
	}
	for t := p.peek(); t != nil && t.kind == "OR"; t = p.peek() {
		p.pos++
		if n := p.parseAnd(); n != nil {
			parts = append(parts, n)
		}
	}
	switch len(parts) {
	case 0:
		return nil
	case 1:
		return parts[0]
	}
	return orNode(parts)
}

func (p *parser) parseAnd() node {
	var parts []node
	for {
		t := p.peek()
		if t == nil || t.kind == "OR" || t.kind == ")" {
			break
		}
		if t.kind == "AND" {
			p.pos++
			continue
		}
		if n := p.parseUnary(); n != nil {
			parts = append(parts, n)
		}
	}
	switch len(parts) {
	case 0:
		return nil
	case 1:
		return parts[0]
	}
	return andNode(parts)
}

func (p *parser) parseUnary() node {
	t := p.peek()
	if t == nil {
		return nil
	}
	switch t.kind {
	case "-", "NOT":
		p.pos++
		n := p.parseUnary()
		if n == nil {
			return nil
		}
		return notNode{n}
	case "(":
		p.pos++
		n := p.parseOr()
		if t := p.peek(); t != nil && t.kind == ")" {
			p.pos++
		}
		return n
	case ")":
		p.pos++
		return nil
	case "quoted":
		p.pos++
		return textNode{t.val}
	}
	p.pos++
	return parseTerm(t.val)
}

func parseTerm(w string) node {
	idx := strings.Index(w, ":")
	if idx <= 0 || idx == len(w)-1 {
		return textNode{w}
	}
	field, value := w[:idx], w[idx+1:]
	// field:(a OR b)
	if strings.HasPrefix(value, "(") && strings.HasSuffix(value, ")") {
		var alts orNode
		for _, v := range strings.Fields(strings.Trim(value, "()")) {
			if v == "OR" {
				continue
			}
			alts = append(alts, fieldNode{field: field, value: strings.Trim(v, `"`)})
		}
		return alts
	}
	for _, op := range []string{">=", "<=", ">", "<"} {
		if strings.HasPrefix(value, op) {
			return fieldNode{field: field, op: op, value: value[len(op):]}
		}
	}
	if (strings.HasPrefix(value, "[") || strings.HasPrefix(value, "{")) && strings.Contains(value, " TO ") {
		inner := value[1 : len(value)-1]
		bounds := strings.SplitN(inner, " TO ", 2)
		return fieldNode{field: field, op: "range", lo: strings.TrimSpace(bounds[0]), hi: strings.TrimSpace(bounds[1])}
	}
	return fieldNode{field: field, value: value}
}

// tagValues extracts the values of `key:value` tags matching key.
func tagValues(tags []string, key string) []string {
	var out []string
	prefix := key + ":"
	for _, t := range tags {
		if strings.HasPrefix(t, prefix) {
			out = append(out, t[len(prefix):])
		}
	}
	return out
}

// lookupAttr resolves a dotted attribute path (http.status_code) in nested maps, also accepting
// flattened keys ("http.status_code" stored literally).
func lookupAttr(attrs map[string]any, name string) (any, bool) {
	if v, ok := attrs[name]; ok {
		return v, true
	}
	parts := strings.Split(name, ".")
	var cur any = attrs
	for i, p := range parts {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		if v, ok := m[p]; ok {
			cur = v
			continue
		}
		// try the rest of the path as a single flattened key
		if v, ok := m[strings.Join(parts[i:], ".")]; ok {
			return v, true
		}
		return nil, false
	}
	return cur, true
}

func valuesOf(v any) []string {
	switch t := v.(type) {
	case nil:
		return nil
	case []any:
		var out []string
		for _, x := range t {
			out = append(out, valuesOf(x)...)
		}
		return out
	case map[string]any:
		return nil
	}
	return []string{asString(v)}
}

// Field implements Event for logs.
func (l *Log) Field(name string) []string {
	switch name {
	case "service":
		return []string{l.Service}
	case "host":
		return []string{l.Host}
	case "status":
		return []string{l.Status}
	case "source":
		return []string{l.Source}
	case "message":
		return []string{l.Message}
	case "trace_id":
		return []string{l.TraceID}
	}
	if strings.HasPrefix(name, "@") {
		v, ok := lookupAttr(l.Attributes, name[1:])
		if !ok {
			return nil
		}
		return valuesOf(v)
	}
	return tagValues(l.Tags, name)
}

// Text implements Event for logs.
func (l *Log) Text() string { return l.Message }

// Field implements Event for spans.
func (sp *Span) Field(name string) []string {
	switch name {
	case "service":
		return []string{sp.Service}
	case "env":
		return []string{sp.Env}
	case "host":
		return []string{sp.Host}
	case "version":
		return []string{sp.Version}
	case "resource_name", "resource":
		return []string{sp.Resource}
	case "operation_name", "name":
		return []string{sp.Name}
	case "span_type", "type":
		return []string{sp.Type}
	case "trace_id":
		return []string{sp.TraceID}
	case "span_id":
		return []string{sp.SpanID}
	case "parent_id":
		return []string{sp.ParentID}
	case "status":
		if sp.Error != 0 {
			return []string{"error"}
		}
		return []string{"ok"}
	case "@duration", "duration":
		return []string{strconv.FormatInt(sp.Duration, 10)}
	case "@_top_level", "_top_level", "is_top_level", "@is_top_level":
		if sp.IsTopLevel {
			return []string{"1", "true"}
		}
		return []string{"0", "false"}
	case "is_root", "@is_root":
		if sp.IsRoot {
			return []string{"1", "true"}
		}
		return []string{"0", "false"}
	}
	key := strings.TrimPrefix(name, "@")
	if v, ok := sp.Meta[key]; ok {
		return []string{v}
	}
	if v, ok := sp.Metrics[key]; ok {
		return []string{strconv.FormatFloat(v, 'f', -1, 64)}
	}
	return nil
}

// Text implements Event for spans.
func (sp *Span) Text() string {
	return sp.Resource + " " + sp.Name + " " + sp.Meta["error.message"]
}

func asString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	}
	return fmt.Sprint(v)
}
