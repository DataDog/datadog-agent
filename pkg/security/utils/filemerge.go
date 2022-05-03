// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// StringPair represents a pair of prefix and suffix strings
// this pair represents a glob with a star between the prefix and the suffix
type StringPair struct {
	left, right string
	isPattern   bool
}

// NewStringPair returns a new StringPair from a string
func NewStringPair(s string) StringPair {
	i := strings.Index(s, "*")
	if i != -1 {
		return StringPair{
			left:      s[:i],
			right:     s[i+1:],
			isPattern: true,
		}
	}

	return StringPair{
		left:  s,
		right: "",
	}
}

// ToGlob returns a glob from the StringPair
func (sp *StringPair) ToGlob() string {
	if sp.isPattern {
		return fmt.Sprintf("%s*%s", sp.left, sp.right)
	}
	return sp.left
}

func commonPrefix(ap, bp StringPair) string {
	a := ap.left
	b := bp.left

	i := 0
	for ; i < len(a) && i < len(b) && a[i] < utf8.RuneSelf && a[i] == b[i]; i++ {
	}

	return a[:i]
}

func commonSuffix(ap, bp StringPair) string {
	a := ap.right
	if !bp.isPattern {
		a = ap.left
	}
	b := bp.right
	if !bp.isPattern {
		b = bp.left
	}

	dec := func(i, j *int) {
		*i--
		*j--
	}

	i := len(a) - 1
	for j := len(b) - 1; i >= 0 && j >= 0 && a[i] < utf8.RuneSelf && a[i] == b[j]; dec(&i, &j) {

	}

	return a[i+1:]
}

// BuildGlob builds a common glob from two string pairs if sufficiently similar
func BuildGlob(ap, bp StringPair, minLenMatch int) (StringPair, bool) {
	p := commonPrefix(ap, bp)
	s := commonSuffix(ap, bp)

	if len(p) < minLenMatch {
		p = ""
	}
	if len(s) < minLenMatch {
		s = ""
	}

	if len(p) == 0 && len(s) == 0 {
		return StringPair{}, false
	}

	return StringPair{left: p, right: s, isPattern: true}, true
}
