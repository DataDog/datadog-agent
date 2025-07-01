// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"fmt"
	"sort"
	"sync"
)

// ProcessingInfo tracks statistics for processing rules applied to a log source
type ProcessingInfo struct {
	rules map[string]int64
	sync.Mutex
}

// NewProcessingInfo creates a new ProcessingRulesInfo instance
func NewProcessingInfo() *ProcessingInfo {
	return &ProcessingInfo{
		rules: make(map[string]int64),
	}
}

// Inc increments the counter for a specific processing rule
func (p *ProcessingInfo) Inc(ruleName string) {
	p.Lock()
	defer p.Unlock()

	p.rules[ruleName]++
}

// GetCount returns the count for a specific processing rule
func (p *ProcessingInfo) GetCount(ruleName string) int64 {
	p.Lock()
	defer p.Unlock()

	return p.rules[ruleName]
}

// Reset resets the processing info
func (p *ProcessingInfo) Reset() {
	p.Lock()
	defer p.Unlock()

	p.rules = make(map[string]int64)
}

// InfoKey returns the key for this info provider
func (p *ProcessingInfo) InfoKey() string {
	return "Processing Rules"
}

// Info returns the processing rules statistics as a slice of strings
func (p *ProcessingInfo) Info() []string {
	p.Lock()
	defer p.Unlock()

	if len(p.rules) == 0 {
		return []string{"No rules applied"}
	}

	info := make([]string, 0, len(p.rules))
	for ruleName, count := range p.rules {
		info = append(info, fmt.Sprintf("Rule [%s] applied to %d log(s)", ruleName, count))
	}
	sort.Strings(info)

	return info
}
