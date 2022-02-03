// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eval

import (
	"strings"

	"github.com/pkg/errors"
)

// Glob describes file glob object
type Glob struct {
	pattern  string
	elements []string
	isScalar bool
}

func (g *Glob) contains(filename string, strict bool) bool {
	if len(g.elements) == 0 || len(filename) == 0 {
		return false
	}

	// normalize */ == /*/
	if g.elements[0] == "*" {
		filename = filename[1:]
	}

	var elp, elf string
	for start, end, i := 0, 0, 0; end != len(filename); end++ {
		if filename[end] == '/' {
			elf, elp = filename[start:end], g.elements[i]
			if !PatternMatches(elp, elf) && elp != "**" {
				return false
			}
			start = end + 1
			i++
		}

		if i+1 > len(g.elements) {
			return !strict || elp == "**"
		}

		if end+1 >= len(filename) {
			elf, elp = filename[start:end+1], g.elements[i]
			if len(elf) == 0 {
				return !strict
			}
			if !PatternMatches(elp, elf) && elp != "**" {
				return false
			}
		}
	}

	return true
}

// Contains returns whether the glob pattern matches the beginning of the filename
func (g *Glob) Contains(filename string) bool {
	return g.contains(filename, false)
}

// Matches the given filename
func (g *Glob) Matches(filename string) bool {
	if g.isScalar {
		return g.pattern == filename
	}
	return g.contains(filename, true)
}

// NewGlob returns a new glob object from the given pattern
func NewGlob(pattern string) (*Glob, error) {
	els := strings.Split(pattern, "/")
	for i, el := range els {
		if el == "**" && i+1 != len(els) || strings.Contains(el, "**") && len(el) != len("**") {
			return nil, errors.New("`**` is allowed only at the end of patterns")
		}
	}

	return &Glob{
		pattern:  pattern,
		elements: els,
		isScalar: !strings.Contains(pattern, "*"),
	}, nil
}
