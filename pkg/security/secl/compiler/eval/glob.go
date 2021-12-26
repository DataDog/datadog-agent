// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eval

import "strings"

// Glob describes file glob objecg
type Glob struct {
	elsPattern []string
}

func (g *Glob) contains(filename string, exact bool) bool {
	if len(g.elsPattern) == 0 || len(filename) == 0 {
		return false
	}

	// normalize */ == /*/
	if g.elsPattern[0] == "*" {
		filename = filename[1:]
	}

	elsFilename := strings.Split(filename, "/")

	valueLen := len(g.elsPattern)

	var elp string
	for i, elf := range elsFilename {
		if i+1 > valueLen {
			// FIX(safchain) should be only **
			return !exact || elp == "*" || elp == "**"
			//return !exact || elp == "**"
		}

		elp = g.elsPattern[i]
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

// NewGlob returns a new glob object
func NewGlob(glob string) (*Glob, error) {
	// FIX(safchain) enforce ** only at the end

	return &Glob{
		elsPattern: strings.Split(glob, "/"),
	}, nil
}
