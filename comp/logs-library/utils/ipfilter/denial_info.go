// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ipfilter

import (
	"fmt"
	"sort"
	"sync"
)

// DenialInfo tracks per-reason denial counts for display in agent status.
// It implements the InfoProvider interface from pkg/logs/status/utils.
type DenialInfo struct {
	mu      sync.Mutex
	reasons map[string]int64
}

// NewDenialInfo creates a new DenialInfo instance.
func NewDenialInfo() *DenialInfo {
	return &DenialInfo{
		reasons: make(map[string]int64),
	}
}

// Record increments the counter for the given denial reason.
func (d *DenialInfo) Record(reason string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.reasons[reason]++
}

// InfoKey returns the label used in agent status output.
func (d *DenialInfo) InfoKey() string {
	return "IP Filter"
}

// Info returns denial statistics as a sorted list of strings.
func (d *DenialInfo) Info() []string {
	d.mu.Lock()
	defer d.mu.Unlock()

	if len(d.reasons) == 0 {
		return []string{"No denials"}
	}

	info := make([]string, 0, len(d.reasons))
	for reason, count := range d.reasons {
		info = append(info, fmt.Sprintf("%s: %d", reason, count))
	}
	sort.Strings(info)
	return info
}
