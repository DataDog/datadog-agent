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
	prefix          []patternElement
	suffix          []patternElement
	isScalar        bool
	caseInsensitive bool
	normalizePaths  bool
}

func (g *Glob) isPrefix(filename string, elements []patternElement) bool {
	if len(elements) == 0 || len(filename) == 0 {
		return false
	}

	// pattern "/"
	if len(elements) == 2 && elements[1].pattern == "" {
		return true
	}

	// normalize */ == /*/
	if elements[0].pattern == "*" {
		filename = filename[1:]
	}

	var (
		elp           patternElement
		elf           string
		start, end, i int = 0, 0, 0
	)

	for ; end != len(filename); end++ {
		if filename[end] == '/' {
			elf, elp := filename[start:end], elements[i]
			if !PatternMatchesWithSegments(elp, elf, g.caseInsensitive) && elp.pattern != "**" {
				return false
			}
			start = end + 1
			i++

			if i == len(elements) {
				return true
			}
		}
	}

	elf, elp = filename[start:end], elements[i]
	if len(elf) == 0 {
		return true
	}
	return PatternMatchesWithSegments(elp, elf, g.caseInsensitive) || elp.pattern == "**"
}

func (g *Glob) matchesPrefix(filename string) bool {
	// normalize */ == /*/
	if g.prefix[0].pattern == "*" {
		filename = filename[1:]
	}

	var (
		elp           patternElement
		elf           string
		start, end, i int = 0, 0, 0
	)

	for ; end != len(filename); end++ {
		if filename[end] == '/' {
			elf, elp = filename[start:end], g.prefix[i]
			if !PatternMatchesWithSegments(elp, elf, g.caseInsensitive) && elp.pattern != "**" {
				return false
			}
			start = end + 1
			i++

			if i == len(g.prefix) {
				return elp.pattern == "**"
			}
		}
	}

	elf, elp = filename[start:], g.prefix[i]
	if elp.pattern == "**" {
		return true
	}

	if len(elf) == 0 && elp.pattern == "*" {
		return true
	}

	return PatternMatchesWithSegments(elp, elf, g.caseInsensitive) && i+1 == len(g.prefix)
}

func (g *Glob) matchesSuffix(filename string) bool {
	var (
		elp           patternElement
		elf           string
		start, end, i = len(filename) - 1, len(filename), len(g.suffix) - 1
	)

	if len(g.suffix) == 1 && g.suffix[0].pattern == "**" {
		return true
	}

	for ; start >= 0; start-- {
		if filename[start] == '/' {
			elf, elp = filename[start+1:end], g.suffix[i]
			if !PatternMatchesWithSegments(elp, elf, g.caseInsensitive) && elp.pattern != "**" {
				return false
			}
			end = start
			i--

			if i == 0 && g.suffix[i].pattern == "**" {
				return true
			}
		}
	}
	elf, elp = filename[:end], g.suffix[i]
	if elp.pattern == "**" {
		return true
	}

	return PatternMatchesWithSegments(elp, elf, g.caseInsensitive) && i == 0
}

// IsPrefix returns whether the glob pattern matches the beginning of the filename
func (g *Glob) IsPrefix(filename string) bool {
	if g.normalizePaths {
		// normalize to linux-like paths
		filename = strings.ReplaceAll(filename, "\\", "/")
	}

	return g.isPrefix(filename, g.prefix)
}

// Matches the given filename
func (g *Glob) Matches(filename string) bool {
	if filename == "" {
		return false
	}

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

	if len(g.prefix) > 0 && !g.matchesPrefix(filename) {
		return false
	} else if len(g.suffix) > 0 && !g.matchesSuffix(filename) {
		return false
	}

	return len(g.prefix) > 0 || len(g.suffix) > 0
}

// NewGlob returns a new glob object from the given pattern
func NewGlob(pattern string, caseInsensitive bool, normalizePaths bool) (*Glob, error) {
	if normalizePaths {
		// normalize to linux-like paths
		pattern = strings.ReplaceAll(pattern, "\\", "/")
	}

	els := strings.Split(pattern, "/")
	elements := make([]patternElement, 0, len(els))

	pos := -1

	for i, el := range els {
		if strings.Contains(el, "**") && len(el) != len("**") {
			// replace ** by * as `**abs**` doesn't make sense
			el = strings.ReplaceAll(el, "**", "*")
		}

		elements = append(elements, newPatternElement(el))

		if el == "**" && pos != -1 {
			return nil, errors.New("`**` is allowed only once in a pattern")
		}

		if el == "**" {
			pos = i
		}
	}

	g := &Glob{
		pattern:         pattern,
		isScalar:        !strings.Contains(pattern, "*"),
		caseInsensitive: caseInsensitive,
		normalizePaths:  normalizePaths,
	}

	if pos == -1 {
		g.prefix = elements
	} else {
		g.prefix = elements[:pos+1]
		g.suffix = elements[pos:]
	}

	return g, nil
}
