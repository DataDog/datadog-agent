// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rules holds rules related files
package rules

import (
	"fmt"
	"io"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

// RuleCoverageReport is the evaluation coverage of a single rule
type RuleCoverageReport struct {
	RuleID    eval.RuleID    `json:"rule_id"`
	EventType eval.EventType `json:"event_type"`
	Coverage  *eval.Coverage `json:"coverage"`
}

// CoverageReport is the evaluation coverage of a whole rule set
type CoverageReport struct {
	// TotalPaths and CoveredPaths sum up the paths of every tracked rule
	TotalPaths   int `json:"total_paths"`
	CoveredPaths int `json:"covered_paths"`
	// TrackedRules and UntrackedRules count the rules whose coverage could, and
	// could not, be tracked
	TrackedRules   int `json:"tracked_rules"`
	UntrackedRules int `json:"untracked_rules"`

	Rules []RuleCoverageReport `json:"rules"`
}

// GetRuleCoverageReport returns the evaluation coverage accumulated by the rule
// set since it was loaded. It returns nil when rule coverage is disabled.
func (rs *RuleSet) GetRuleCoverageReport() *CoverageReport {
	if !rs.evalOpts.RuleCoverage {
		return nil
	}

	report := &CoverageReport{}

	for _, rule := range rs.GetRules() {
		coverage := rule.GetCoverage()
		if coverage == nil {
			continue
		}

		// the event type is known to be resolvable, the rule would not have been
		// added to the rule set otherwise
		eventType, _ := rule.GetEventType()

		ruleReport := coverage.Report()
		if ruleReport.Unsupported != "" {
			report.UntrackedRules++
		} else {
			report.TrackedRules++
			report.TotalPaths += ruleReport.TotalPaths
			report.CoveredPaths += ruleReport.CoveredPaths
		}

		report.Rules = append(report.Rules, RuleCoverageReport{
			RuleID:    rule.ID,
			EventType: eventType,
			Coverage:  ruleReport,
		})
	}

	return report
}

// ResetRuleCoverage drops the coverage accumulated so far by every rule
func (rs *RuleSet) ResetRuleCoverage() {
	for _, rule := range rs.GetRules() {
		if coverage := rule.GetCoverage(); coverage != nil {
			coverage.Reset()
		}
	}
}

// Render writes the report in a human readable form
func (r *CoverageReport) Render(writer io.Writer) error {
	percentage := 100.0
	if r.TotalPaths > 0 {
		percentage = 100 * float64(r.CoveredPaths) / float64(r.TotalPaths)
	}

	if _, err := fmt.Fprintf(writer, "Rule coverage: %d/%d paths (%.1f%%) over %d rules",
		r.CoveredPaths, r.TotalPaths, percentage, r.TrackedRules); err != nil {
		return err
	}
	if r.UntrackedRules > 0 {
		if _, err := fmt.Fprintf(writer, ", %d rule(s) not tracked", r.UntrackedRules); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(writer); err != nil {
		return err
	}

	var eventType eval.EventType
	for _, rule := range r.Rules {
		if rule.EventType != eventType {
			eventType = rule.EventType
			if _, err := fmt.Fprintf(writer, "\n%s\n", eventType); err != nil {
				return err
			}
		}

		if err := renderRuleCoverage(writer, rule); err != nil {
			return err
		}
	}

	return nil
}

func renderRuleCoverage(writer io.Writer, rule RuleCoverageReport) error {
	coverage := rule.Coverage

	if coverage.Unsupported != "" {
		_, err := fmt.Fprintf(writer, "  %s: not tracked (%s)\n", rule.RuleID, coverage.Unsupported)
		return err
	}

	if _, err := fmt.Fprintf(writer, "  %s: %d/%d paths, %d evaluation(s)\n",
		rule.RuleID, coverage.CoveredPaths, coverage.TotalPaths, coverage.Evaluations); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(writer, "    %s\n", coverage.Skeleton); err != nil {
		return err
	}

	for _, leaf := range coverage.Leaves {
		if _, err := fmt.Fprintf(writer, "      %s = %s  (true: %d, false: %d)\n",
			leaf.Name, leaf.Expression, leaf.True, leaf.False); err != nil {
			return err
		}
	}

	// pad the condition lists so that the outcomes line up
	width := 0
	conditions := make([]string, len(coverage.Paths))
	for i, path := range coverage.Paths {
		conditions[i] = path.String()
		if len(conditions[i]) > width {
			width = len(conditions[i])
		}
	}

	for i, path := range coverage.Paths {
		mark := " "
		if path.Hits > 0 {
			mark = "x"
		}
		if _, err := fmt.Fprintf(writer, "    [%s] %10d  %-*s => %t\n",
			mark, path.Hits, width, conditions[i], path.Result); err != nil {
			return err
		}
	}

	return nil
}

// String returns the report in a human readable form
func (r *CoverageReport) String() string {
	var builder strings.Builder
	if err := r.Render(&builder); err != nil {
		return fmt.Sprintf("failed to render the rule coverage report: %s", err)
	}
	return builder.String()
}
