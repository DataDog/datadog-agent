// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package yara

import (
	"embed"
	"fmt"
	"sort"
)

// embeddedRulesFS holds the built-in YARA rule set that ships with the binary. These are
// community rules vetted from YARAify (abuse.ch); see rules/README for provenance and licensing.
// Only the files that compile cleanly against libyara are embedded (see rules_compilecheck_test.go).
//
//go:embed all:rules
var embeddedRulesFS embed.FS

// embeddedRulesDir is the directory inside embeddedRulesFS holding the rule files
const embeddedRulesDir = "rules"

// EmbeddedRuleSources returns the rule files embedded in the binary, sorted by name. These are the
// default rule set of the YARA exec scanner.
func EmbeddedRuleSources() ([]RuleSource, error) {
	entries, err := embeddedRulesFS.ReadDir(embeddedRulesDir)
	if err != nil {
		return nil, fmt.Errorf("yara: failed to read embedded rules: %w", err)
	}
	var sources []RuleSource
	for _, entry := range entries {
		if entry.IsDir() || !isRuleFileName(entry.Name()) {
			continue
		}
		data, err := embeddedRulesFS.ReadFile(embeddedRulesDir + "/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("yara: failed to read embedded rule %s: %w", entry.Name(), err)
		}
		sources = append(sources, RuleSource{Name: entry.Name(), Data: data})
	}
	sort.Slice(sources, func(i, j int) bool { return sources[i].Name < sources[j].Name })
	return sources, nil
}
