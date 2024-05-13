// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package model holds model related files
package model

import (
	"slices"
	"strings"
)

const (
	// MaxArgEnvSize maximum size of one argument or environment variable
	MaxArgEnvSize = 256
	// MaxArgsEnvsSize maximum number of args and/or envs
	MaxArgsEnvsSize = 256
)

// ArgsEnvs raw value for args and envs
type ArgsEnvs struct {
	ID        uint32
	Size      uint32
	ValuesRaw [MaxArgEnvSize]byte
}

// ArgsEntry defines a args cache entry
type ArgsEntry struct {
	Values    []string
	Truncated bool
}

// Equals compares two ArgsEntry
func (p *ArgsEntry) Equals(o *ArgsEntry) bool {
	if p == o {
		return true
	} else if p == nil || o == nil {
		return false
	}

	return slices.Equal(p.Values, o.Values)
}

// EnvsEntry defines a args cache entry
type EnvsEntry struct {
	Values    []string
	Truncated bool

	filteredEnvs []string
	kv           map[string]string
}

// FilterEnvs returns an array of envs, only the name of each variable is returned unless the variable name is part of the provided filter
func (p *EnvsEntry) FilterEnvs(envsWithValue map[string]bool) ([]string, bool) {
	if p.filteredEnvs != nil {
		return p.filteredEnvs, p.Truncated
	}

	if len(p.Values) == 0 {
		return nil, p.Truncated
	}

	p.filteredEnvs = make([]string, 0, len(p.Values))

	for _, value := range p.Values {
		k, _, found := strings.Cut(value, "=")
		if found {
			if envsWithValue[k] {
				p.filteredEnvs = append(p.filteredEnvs, value)
			} else {
				p.filteredEnvs = append(p.filteredEnvs, k)
			}
		} else {
			p.filteredEnvs = append(p.filteredEnvs, value)
		}
	}

	return p.filteredEnvs, p.Truncated
}

// FilterEnvs returns an array of environment variable key value pairs matching the desired keys
func FilterEnvs(allEnvVars []string, desiredKeys map[string]bool) []string {
	if len(allEnvVars) == 0 {
		return nil
	}

	filteredEnvs := make([]string, 0, len(desiredKeys))

	for _, value := range allEnvVars {
		k, _, _ := strings.Cut(value, "=")
		if desiredKeys[k] {
			filteredEnvs = append(filteredEnvs, value)
		}
	}

	return filteredEnvs
}

func (p *EnvsEntry) toMap() {
	if p.kv != nil {
		return
	}

	p.kv = make(map[string]string, len(p.Values))

	for _, value := range p.Values {
		k, v, found := strings.Cut(value, "=")
		if found {
			p.kv[k] = v
		}
	}
}

// Get returns the value for the given key
func (p *EnvsEntry) Get(key string) string {
	p.toMap()
	return p.kv[key]
}

// Equals compares two EnvsEntry
func (p *EnvsEntry) Equals(o *EnvsEntry) bool {
	if p == o {
		return true
	} else if p == nil || o == nil {
		return false
	}

	return slices.Equal(p.Values, o.Values)
}
