// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eval holds eval related files
package eval

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// RegisterCacheEntry used to track the value
type RegisterCacheEntry struct {
	Pos   int
	Value interface{}
}

// MatchingSubExpr defines a boolean expression that matched
type MatchingSubExpr struct {
	Offset int           `json:"offset"`
	ValueA MatchingValue `json:"code"`
	ValueB MatchingValue
}

// MatchingValue defines a matched value
type MatchingValue struct {
	Offset int
	Field  Field
	Value  interface{}
}

// MatchingValuePos defines a position and a length in the rule expression
type MatchingValuePos struct {
	Offset int
	Length int
}

// Context describes the context used during a rule evaluation
type Context struct {
	Event Event

	// cache available across all the evaluations
	StringCache map[Field][]string
	IPNetCache  map[Field][]net.IPNet
	IntCache    map[Field][]int
	BoolCache   map[Field][]bool

	// iterator register cache. used to cache entry within a single rule evaluation
	RegisterCache map[RegisterID]*RegisterCacheEntry

	// rule register
	Registers map[RegisterID]int

	IteratorCountCache map[string]int

	// internal
	now              time.Time
	resolvedFields   []string
	matchingSubExprs []MatchingSubExpr
}

// Now return and cache the `now` timestamp
func (c *Context) Now() time.Time {
	if c.now.IsZero() {
		c.now = time.Now()
	}
	return c.now
}

// SetEvent set the given event to the context
func (c *Context) SetEvent(evt Event) {
	c.Event = evt
}

// Reset the context
func (c *Context) Reset() {
	c.Event = nil
	c.now = time.Time{}

	clear(c.StringCache)
	clear(c.IPNetCache)
	clear(c.IntCache)
	clear(c.BoolCache)
	clear(c.Registers)
	clear(c.RegisterCache)
	clear(c.IteratorCountCache)
	c.resolvedFields = nil
	clear(c.matchingSubExprs)

	// per eval
	c.PerEvalReset()
}

// PerEvalReset the context
func (c *Context) PerEvalReset() {
	c.matchingSubExprs = c.matchingSubExprs[0:0]
}

// GetResolvedFields returns the resolved fields, always empty outside of functional tests
func (c *Context) GetResolvedFields() []Field {
	return c.resolvedFields
}

// AddMatchingSubExpr add a expression that matched a rule
func (c *Context) AddMatchingSubExpr(valueA, valueB MatchingValue) {
	if valueA.Field == "" && valueB.Field == "" {
		return
	}

	subExprOffset := valueA.Offset
	if valueB.Offset > 0 && valueB.Offset < subExprOffset {
		subExprOffset = valueB.Offset
	}

	c.matchingSubExprs = append(c.matchingSubExprs, MatchingSubExpr{
		Offset: subExprOffset,
		ValueA: valueA,
		ValueB: valueB,
	})
}

// MatchingSubExprs list of sub expression
type MatchingSubExprs []MatchingSubExpr

// GetMatchingSubExpr add an expression that matched a rule
func (c *Context) GetMatchingSubExprs() MatchingSubExprs {
	return c.matchingSubExprs
}

// IsZero returns if the pos is empty of not
func (m MatchingValuePos) IsZero() bool {
	return m.Length == 0
}

func (m *MatchingValue) getPosWithinRuleExpr(expr string, offset int) MatchingValuePos {
	var pos MatchingValuePos

	// take the more accurate offset to start with
	if m.Offset > offset {
		offset = m.Offset
	}

	if offset >= len(expr) {
		return pos
	}

	if m.Field != "" {
		if idx := strings.Index(expr[offset:], m.Field); idx >= 0 {
			pos.Offset = idx + offset
			pos.Length = len(m.Field)
		}
	} else {
		var str string

		switch value := m.Value.(type) {
		case string:
			str = value
		case int:
			str = strconv.Itoa(value)
		case bool:
			str = "false"
			if value {
				str = "true"
			}
		case net.IPNet, *net.IPNet:
			var ip, cidr string
			if v, ok := value.(net.IPNet); ok {
				ip, cidr = v.IP.String(), v.String()
			} else if v, ok := value.(*net.IPNet); ok {
				ip, cidr = v.IP.String(), v.String()
			}

			if idx := strings.Index(expr[offset:], ip); idx >= 0 {
				pos.Offset = idx + offset
				if strings.Index(expr[offset+idx:], "/") > 0 {
					pos.Length = len(cidr)
				} else {
					pos.Length = len(ip)
				}
			}
			return pos
		case []*net.IPNet:
			var startOffset, length int
			for _, n := range value {
				ip, cidr := n.IP.String(), n.String()

				if idx := strings.Index(expr[offset:], ip); idx >= 0 {
					if startOffset == 0 {
						startOffset = idx + offset
					}
					if strings.Index(expr[offset+idx:], "/") > 0 {
						length = len(cidr)
					} else {
						length = len(ip)
					}
					offset += idx + length
				}
			}

			pos.Offset = startOffset
			pos.Length = offset - startOffset

			return pos
		case fmt.Stringer:
			str = value.String()
		default:
			return pos
		}
		if idx := strings.Index(expr[offset:], str); idx >= 0 {
			pos.Offset = idx + offset
			pos.Length = len(str)
		}
	}

	return pos
}

// DecorateRuleExpr decorate the rule
func (m *MatchingSubExpr) DecorateRuleExpr(expr string, before, after string) (string, error) {
	a, b := m.ValueA.getPosWithinRuleExpr(expr, m.Offset), m.ValueB.getPosWithinRuleExpr(expr, m.Offset)

	if a.Offset+a.Length > len(expr) || b.Offset+b.Length > len(expr) {
		return expr, errors.New("expression overflow")
	}

	if b.Offset < a.Offset {
		tmp := b
		b = a
		a = tmp
	}

	if a.Length == 0 {
		return expr[:b.Offset] + before + expr[b.Offset:b.Offset+b.Length] + after + expr[b.Offset+b.Length:], nil
	}

	if b.Length == 0 {
		return expr[0:a.Offset] + before + expr[a.Offset:a.Offset+a.Length] + after + expr[a.Offset+a.Length:], nil
	}

	return expr[0:a.Offset] + before + expr[a.Offset:a.Offset+a.Length] + after +
		expr[a.Offset+a.Length:b.Offset] + before + expr[b.Offset:b.Offset+b.Length] + after +
		expr[b.Offset+b.Length:], nil
}

// GetMatchingValuePos return all the matching value position
func (m *MatchingSubExprs) GetMatchingValuePos(expr string) []MatchingValuePos {
	var pos []MatchingValuePos

	for _, mse := range *m {
		a, b := mse.ValueA.getPosWithinRuleExpr(expr, mse.Offset), mse.ValueB.getPosWithinRuleExpr(expr, mse.Offset)
		if !a.IsZero() {
			pos = append(pos, a)
		}
		if !b.IsZero() {
			pos = append(pos, b)
		}
	}

	return pos
}

// DecorateRuleExpr decorate the rule
func (m *MatchingSubExprs) DecorateRuleExpr(expr string, before, after string) (string, error) {
	var err error
	for _, mse := range *m {
		expr, err = mse.DecorateRuleExpr(expr, before, after)
		if err != nil {
			return expr, err
		}
	}
	return expr, nil
}

// NewContext return a new Context
func NewContext(evt Event) *Context {
	return &Context{
		Event:              evt,
		StringCache:        make(map[Field][]string),
		IPNetCache:         make(map[Field][]net.IPNet),
		IntCache:           make(map[Field][]int),
		BoolCache:          make(map[Field][]bool),
		Registers:          make(map[RegisterID]int),
		RegisterCache:      make(map[RegisterID]*RegisterCacheEntry),
		IteratorCountCache: make(map[string]int),
	}
}

// ContextPool defines a pool of context
type ContextPool struct {
	pool sync.Pool
}

// Get returns a context with the given event
func (c *ContextPool) Get(evt Event) *Context {
	ctx := c.pool.Get().(*Context)
	ctx.SetEvent(evt)
	return ctx
}

// Put returns the context to the pool
func (c *ContextPool) Put(ctx *Context) {
	ctx.Reset()
	c.pool.Put(ctx)
}

// NewContextPool returns a new context pool
func NewContextPool() *ContextPool {
	return &ContextPool{
		pool: sync.Pool{
			New: func() interface{} { return NewContext(nil) },
		},
	}
}
