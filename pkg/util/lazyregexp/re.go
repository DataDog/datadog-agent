// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package lazyregexp holds lazy initliazed regexp code
package lazyregexp

import (
	"regexp"
	"sync"
)

var inTest bool

// LazyRegexp is a lazy initialized regexp
type LazyRegexp struct {
	expression string
	once       sync.Once
	compiled   *regexp.Regexp
}

// New returns a new LazyRegexp
func New(expression string) *LazyRegexp {
	lr := &LazyRegexp{
		expression: expression,
	}

	// in test code, we force the regexp to be compiled at creation time
	// to make sure the pattern is valid
	if inTest {
		_ = lr.re()
	}

	return lr
}

func (lr *LazyRegexp) re() *regexp.Regexp {
	lr.once.Do(func() {
		lr.compiled = regexp.MustCompile(lr.expression)
	})
	return lr.compiled
}

// Match see regexp.Match
func (lr *LazyRegexp) Match(b []byte) bool {
	return lr.re().Match(b)
}

// MatchString see regexp.MatchString
func (lr *LazyRegexp) MatchString(s string) bool {
	return lr.re().MatchString(s)
}

// FindStringSubmatch see regexp.FindStringSubmatch
func (lr *LazyRegexp) FindStringSubmatch(s string) []string {
	return lr.re().FindStringSubmatch(s)
}

// ReplaceAllString see regexp.ReplaceAllString
func (lr *LazyRegexp) ReplaceAllString(src, repl string) string {
	return lr.re().ReplaceAllString(src, repl)
}

// FindSubmatch see regexp.FindSubmatch
func (lr *LazyRegexp) FindSubmatch(s []byte) [][]byte {
	return lr.re().FindSubmatch(s)
}

// FindAllStringSubmatch see regexp.FindAllStringSubmatch
func (lr *LazyRegexp) FindAllStringSubmatch(s string, n int) [][]string {
	return lr.re().FindAllStringSubmatch(s, n)
}

// FindAllStringSubmatchIndex see regexp.FindAllStringSubmatchIndex
func (lr *LazyRegexp) FindAllStringSubmatchIndex(s string, n int) [][]int {
	return lr.re().FindAllStringSubmatchIndex(s, n)
}

// FindIndex see regexp.FindIndex
func (lr *LazyRegexp) FindIndex(b []byte) []int {
	return lr.re().FindIndex(b)
}

// FindAll see regexp.FindAll
func (lr *LazyRegexp) FindAll(b []byte, n int) [][]byte {
	return lr.re().FindAll(b, n)
}

// FindString see regexp.FindString
func (lr *LazyRegexp) FindString(s string) string {
	return lr.re().FindString(s)
}
