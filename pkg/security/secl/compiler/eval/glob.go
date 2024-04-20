// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eval holds eval related files
package eval

import (
	"errors"
	"strings"
)

// Glob describes file glob object
type Glob struct {
	pattern         string
	elements        []patternElement
	isScalar        bool
	caseInsensitive bool
	normalizePaths  bool
}

func (g *Glob) contains(filename string) bool {
	if len(g.elements) == 0 || len(filename) == 0 {
		return false
	}

	// pattern "/"
	if len(g.elements) == 2 && g.elements[1].pattern == "" {
		return true
	}

	// normalize */ == /*/
	if g.elements[0].pattern == "*" {
		filename = filename[1:]
	}

	for start, end, i := 0, 0, 0; end != len(filename); end++ {
		if filename[end] == '/' {
			elf, elp := filename[start:end], g.elements[i]
			if !PatternMatchesWithSegments(elp, elf, g.caseInsensitive) && elp.pattern != "**" {
				return false
			}
			start = end + 1
			i++
		}

		if i+1 > len(g.elements) {
			return true
		}

		if end+1 >= len(filename) {
			elf, elp := filename[start:end+1], g.elements[i]
			if len(elf) == 0 {
				return true
			}
			if !PatternMatchesWithSegments(elp, elf, g.caseInsensitive) && elp.pattern != "**" {
				return false
			}
		}
	}

	return true
}

func (g *Glob) matches(filename string) bool {
	if len(g.elements) == 0 || len(filename) == 0 {
		return false
	}

	// normalize */ == /*/
	if g.elements[0].pattern == "*" {
		filename = filename[1:]
	}

	var elp patternElement
	var elf string
	var start, end, i int

	for start, end, i = 0, 0, 0; end != len(filename); end++ {
		if filename[end] == '/' {
			elf, elp = filename[start:end], g.elements[i]
			if !PatternMatchesWithSegments(elp, elf, g.caseInsensitive) && elp.pattern != "**" {
				return false
			}
			start = end + 1
			i++
		}

		if i+1 > len(g.elements) {
			return elp.pattern == "**"
		}

		if end+1 >= len(filename) {
			elf, elp = filename[start:end+1], g.elements[i]
			if len(elf) == 0 {
				return elp.pattern == "*"
			}
			if PatternMatchesWithSegments(elp, elf, g.caseInsensitive) && i+1 == len(g.elements) {
				return true
			} else if elp.pattern != "**" {
				return false
			}
		}
	}

	elf, elp = filename[end:], g.elements[i+1]
	if len(elf) == 0 {
		return false
	}
	return PatternMatchesWithSegments(elp, elf, g.caseInsensitive)
}

// Contains returns whether the glob pattern matches the beginning of the filename
func (g *Glob) Contains(filename string) bool {
	if g.normalizePaths {
		// normalize to linux-like paths
		filename = strings.ReplaceAll(filename, "\\", "/")
	}

	return g.contains(filename)
}

// Matches the given filename
func (g *Glob) Matches(filename string) bool {
	if g.normalizePaths {
		// normalize to linux-like paths
		filename = strings.ReplaceAll(filename, "\\", "/")
	}

	if g.isScalar {
		if g.caseInsensitive {
			return strings.EqualFold(g.pattern, filename)
		}
		return g.pattern == filename
	}
	return g.matches(filename)
}

// NewGlob returns a new glob object from the given pattern
func NewGlob(pattern string, caseInsensitive bool, normalizePaths bool) (*Glob, error) {
	if normalizePaths {
		// normalize to linux-like paths
		pattern = strings.ReplaceAll(pattern, "\\", "/")
	}

	els := strings.Split(pattern, "/")
	elements := make([]patternElement, 0, len(els))
	for i, el := range els {
		if el == "**" && i+1 != len(els) || strings.Contains(el, "**") && len(el) != len("**") {
			return nil, errors.New("`**` is allowed only at the end of patterns")
		}
		elements = append(elements, newPatternElement(el))
	}

	return &Glob{
		pattern:         pattern,
		elements:        elements,
		isScalar:        !strings.Contains(pattern, "*"),
		caseInsensitive: caseInsensitive,
		normalizePaths:  normalizePaths,
	}, nil
}
