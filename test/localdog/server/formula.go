// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/DataDog/datadog-agent/test/localdog/store"
)

// evalFormula evaluates a formula such as `query1`, `query1 / query2 * 100` or `(query1 + 5) / 2`.
// Grouped operands are matched on their group tags; an ungrouped single series is broadcast.
func evalFormula(formula string, results map[string]*store.MetricQueryResult, n int) ([]*store.MetricResult, error) {
	p := &formulaParser{src: formula}
	p.next()
	v, err := p.expr(results, n)
	if err != nil {
		return nil, err
	}
	if p.tok != "" {
		return nil, fmt.Errorf("unexpected %q in formula %q", p.tok, formula)
	}
	return v.series, nil
}

type fvalue struct {
	series []*store.MetricResult
	scalar *float64
}

type formulaParser struct {
	src string
	pos int
	tok string
}

func (p *formulaParser) next() {
	for p.pos < len(p.src) && p.src[p.pos] == ' ' {
		p.pos++
	}
	if p.pos >= len(p.src) {
		p.tok = ""
		return
	}
	c := rune(p.src[p.pos])
	if strings.ContainsRune("+-*/()", c) {
		p.tok = string(c)
		p.pos++
		return
	}
	start := p.pos
	for p.pos < len(p.src) {
		c := rune(p.src[p.pos])
		if !(unicode.IsLetter(c) || unicode.IsDigit(c) || c == '_' || c == '.') {
			break
		}
		p.pos++
	}
	if start == p.pos {
		p.tok = string(c)
		p.pos++
		return
	}
	p.tok = p.src[start:p.pos]
}

func (p *formulaParser) expr(res map[string]*store.MetricQueryResult, n int) (fvalue, error) {
	left, err := p.term(res, n)
	if err != nil {
		return left, err
	}
	for p.tok == "+" || p.tok == "-" {
		op := p.tok
		p.next()
		right, err := p.term(res, n)
		if err != nil {
			return left, err
		}
		left = combine(left, right, op, n)
	}
	return left, nil
}

func (p *formulaParser) term(res map[string]*store.MetricQueryResult, n int) (fvalue, error) {
	left, err := p.factor(res, n)
	if err != nil {
		return left, err
	}
	for p.tok == "*" || p.tok == "/" {
		op := p.tok
		p.next()
		right, err := p.factor(res, n)
		if err != nil {
			return left, err
		}
		left = combine(left, right, op, n)
	}
	return left, nil
}

func (p *formulaParser) factor(res map[string]*store.MetricQueryResult, n int) (fvalue, error) {
	switch tok := p.tok; {
	case tok == "(":
		p.next()
		v, err := p.expr(res, n)
		if err != nil {
			return v, err
		}
		if p.tok != ")" {
			return v, errors.New("missing closing parenthesis")
		}
		p.next()
		return v, nil
	case tok == "-":
		p.next()
		v, err := p.factor(res, n)
		if err != nil {
			return v, err
		}
		minusOne := -1.0
		return combine(fvalue{scalar: &minusOne}, v, "*", n), nil
	case tok == "":
		return fvalue{}, errors.New("unexpected end of formula")
	default:
		p.next()
		if f, err := strconv.ParseFloat(tok, 64); err == nil {
			return fvalue{scalar: &f}, nil
		}
		r, ok := res[tok]
		if !ok {
			return fvalue{}, fmt.Errorf("unknown query %q in formula", tok)
		}
		return fvalue{series: r.Series}, nil
	}
}

func apply(a, b float64, op string) (float64, bool) {
	switch op {
	case "+":
		return a + b, true
	case "-":
		return a - b, true
	case "*":
		return a * b, true
	case "/":
		if b == 0 {
			return 0, false
		}
		return a / b, true
	}
	return 0, false
}

func mapSeries(s *store.MetricResult, n int, fn func(float64) (float64, bool)) *store.MetricResult {
	out := &store.MetricResult{GroupTags: s.GroupTags, Unit: s.Unit, Values: make([]*float64, n)}
	for i := 0; i < n && i < len(s.Values); i++ {
		if s.Values[i] == nil {
			continue
		}
		if v, ok := fn(*s.Values[i]); ok {
			out.Values[i] = &v
		}
	}
	return out
}

func combine(a, b fvalue, op string, n int) fvalue {
	switch {
	case a.scalar != nil && b.scalar != nil:
		v, _ := apply(*a.scalar, *b.scalar, op)
		return fvalue{scalar: &v}
	case a.scalar != nil:
		out := make([]*store.MetricResult, len(b.series))
		for i, s := range b.series {
			out[i] = mapSeries(s, n, func(x float64) (float64, bool) { return apply(*a.scalar, x, op) })
		}
		return fvalue{series: out}
	case b.scalar != nil:
		out := make([]*store.MetricResult, len(a.series))
		for i, s := range a.series {
			out[i] = mapSeries(s, n, func(x float64) (float64, bool) { return apply(x, *b.scalar, op) })
		}
		return fvalue{series: out}
	}
	key := func(s *store.MetricResult) string { return strings.Join(s.GroupTags, ",") }
	bByKey := map[string]*store.MetricResult{}
	for _, s := range b.series {
		bByKey[key(s)] = s
	}
	var broadcast *store.MetricResult
	if len(b.series) == 1 {
		broadcast = b.series[0]
	}
	var out []*store.MetricResult
	for _, sa := range a.series {
		sb, ok := bByKey[key(sa)]
		if !ok {
			sb = broadcast
		}
		if sb == nil {
			continue
		}
		res := &store.MetricResult{GroupTags: sa.GroupTags, Unit: sa.Unit, Values: make([]*float64, n)}
		for i := 0; i < n; i++ {
			if i >= len(sa.Values) || i >= len(sb.Values) || sa.Values[i] == nil || sb.Values[i] == nil {
				continue
			}
			if v, ok := apply(*sa.Values[i], *sb.Values[i], op); ok {
				res.Values[i] = &v
			}
		}
		out = append(out, res)
	}
	if len(a.series) == 1 && len(b.series) > 1 {
		// broadcast the left side instead
		out = nil
		for _, sb := range b.series {
			res := &store.MetricResult{GroupTags: sb.GroupTags, Unit: a.series[0].Unit, Values: make([]*float64, n)}
			for i := 0; i < n; i++ {
				av, bv := a.series[0].Values, sb.Values
				if i >= len(av) || i >= len(bv) || av[i] == nil || bv[i] == nil {
					continue
				}
				if v, ok := apply(*av[i], *bv[i], op); ok {
					res.Values[i] = &v
				}
			}
			out = append(out, res)
		}
	}
	return fvalue{series: out}
}
