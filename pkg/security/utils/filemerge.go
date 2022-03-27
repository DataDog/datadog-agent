// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"fmt"
	"unicode/utf8"
)

type StringPair struct {
	left, right string
}

func NewStringPair(s string) StringPair {
	return StringPair{
		left:  s,
		right: "",
	}
}

func (sp *StringPair) ToGlob() string {
	if sp.right == "" {
		return sp.left
	}
	return fmt.Sprintf("%s*%s", sp.left, sp.right)
}

func CommonPrefix(ap, bp StringPair) string {
	prefix := make([]byte, 0)

	a := ap.left
	b := bp.left

	for i := 0; i < len(a) && i < len(b) && a[i] < utf8.RuneSelf && a[i] == b[i]; i++ {
		prefix = append(prefix, a[i])
	}

	return string(prefix)
}

func CommonSuffix(ap, bp StringPair) string {
	a := ap.right
	if a == "" {
		a = ap.left
	}
	b := bp.right
	if b == "" {
		b = bp.left
	}

	dec := func(i, j *int) {
		*i--
		*j--
	}

	i := len(a) - 1
	for j := len(b) - 1; i >= 0 && j >= 0 && a[i] < utf8.RuneSelf && a[i] == b[j]; dec(&i, &j) {

	}

	return string(a[i+1:])
}

const MinLenMatch = 4

func BuildGlob(ap, bp StringPair) (StringPair, bool) {
	p := CommonPrefix(ap, bp)
	s := CommonSuffix(ap, bp)

	if len(p) < MinLenMatch {
		p = ""
	}
	if len(s) < MinLenMatch {
		s = ""
	}

	if len(p) == 0 && len(s) == 0 {
		return StringPair{}, false
	}

	return StringPair{left: p, right: s}, true
}

func Combine(inputs []string) []StringPair {
	if len(inputs) == 0 {
		return nil
	}

	current := []StringPair{NewStringPair(inputs[0])}
	for _, a := range inputs[1:] {
		next := make([]StringPair, 0)
		for _, bp := range current {
			ap := NewStringPair(a)
			sp, similar := BuildGlob(ap, bp)
			if similar {
				next = append(next, sp)
			} else {
				next = append(next, ap, bp)
			}
		}
		current = next
	}
	return current
}
