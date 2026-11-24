// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kfilters

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

// FilePattern approves the events on the pattern their file name matches.
type FilePattern struct {
	Pattern string
}

// Weight returns how selective the approver is
func (FilePattern) Weight() int { return 0 }

// ApproverOnly returns whether the events it approves bypass the discarders
func (FilePattern) ApproverOnly() bool { return false }

// filePatterns approves a file name on its value, read as a pattern
var filePatterns = rules.ApproverModel(func(c rules.Comparison) ([]rules.Approver, bool) {
	pattern, ok := c.Equal()
	if !ok {
		pattern, ok = c.Glob()
	}
	if !ok {
		return nil, false
	}
	return []rules.Approver{FilePattern{Pattern: pattern}}, true
})

func init() {
	for _, event := range []string{"create", "rename", "delete", "write"} {
		models[event] = rules.On(filePatterns, event+".file.name")
	}
}
