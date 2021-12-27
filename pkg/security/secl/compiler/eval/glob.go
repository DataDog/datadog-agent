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
	elements []string
}

func (g *Glob) contains(filename string, strict bool) bool {
	if len(g.elements) == 0 || len(filename) == 0 {
		return false
	}

	// normalize */ == /*/
	if g.elements[0] == "*" {
		filename = filename[1:]
	}

	elsFilename := strings.Split(filename, "/")

	valueLen := len(g.elements)

	var elp string
	for i, elf := range elsFilename {
		if i+1 > valueLen {
			return !strict || elp == "**"
		}

		elp = g.elements[i]
		if !PatternMatches(elp, elf) && elp != "**" {
			return false
		}
	}

	return true
}

// Contains the given filename
func (g *Glob) Contains(filename string) bool {
	return g.contains(filename, false)
}

// Matches the given filename
func (g *Glob) Matches(filename string) bool {
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
		elements: els,
	}, nil
}
