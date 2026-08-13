// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/google/cel-go/cel"
	"gopkg.in/yaml.v3"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclcel"
)

// How much of a real rule set can be evaluated through CEL, measured rather than
// argued.
//
// A rule passes through four stages before it can run — translate, type-check,
// resolve its field reads, plan — and each is a different kind of gap: a construct
// the front end does not cover, a name the environment does not declare, a pattern
// that does not compile. Reporting the stage a rule stopped at is what says which
// gap to close next.
//
// Then there is a fifth stage the rules themselves provide. Datadog's agent rules
// carry their own test cases — an event type, a set of field values, and the verdict
// expected — 670 of them over 184 of the 319 linux rules. Building each event and
// evaluating it through *both* engines turns coverage into a differential: not
// "does it compile" but "does it answer what production answers". Every divergence
// recorded in the seclcel package doc was found by hand; this is what would have
// found them.

// policySet is a directory of agent rules and macros, as the security-monitoring
// repository lays them out: one YAML file per rule, one `.macro` file per macro.
type policySet struct {
	macros []seclcel.Macro
	rules  []policyRule
}

type policyRule struct {
	name       string
	file       string
	expression string
	tests      []ruleTest
}

// ruleTest is a case a rule declares for itself.
type ruleTest struct {
	description string
	eventType   string
	values      map[string]any
	expected    bool
}

// ruleFile is the part of a rule's YAML this reads. The rest — tags, rate limits,
// the backend's own bookkeeping — says nothing about whether the expression can be
// evaluated.
type ruleFile struct {
	Type       string `yaml:"type"`
	Name       string `yaml:"name"`
	Expression string `yaml:"expression"`
	OSFilter   string `yaml:"osFilter"`
	Tests      []struct {
		Description string `yaml:"description"`
		Data        struct {
			Type   string         `yaml:"type"`
			Values map[string]any `yaml:"values"`
		} `yaml:"data"`
		Expected bool `yaml:"expected"`
	} `yaml:"tests"`
}

type macroFile struct {
	ID         string `yaml:"id"`
	Expression string `yaml:"expression"`
}

// readPolicies walks a directory of agent rules.
//
// Only linux rules are read, because the environment this measures against carries
// the unix model: a windows rule would be reported as unsupported for naming fields
// that are simply not in it, which would be a misleading number rather than a gap.
func readPolicies(dir string) (policySet, error) {
	var set policySet

	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}

		switch filepath.Ext(path) {
		case ".macro":
			var macro macroFile
			if err := readYAML(path, &macro); err != nil {
				return err
			}
			if macro.ID != "" {
				set.macros = append(set.macros, seclcel.Macro{ID: macro.ID, Expression: macro.Expression})
			}

		case ".yaml", ".yml":
			var rule ruleFile
			if err := readYAML(path, &rule); err != nil {
				return err
			}
			if rule.Type != "agent_rule" || rule.Expression == "" {
				return nil
			}
			if rule.OSFilter == "windows" {
				return nil
			}

			parsed := policyRule{
				name:       rule.Name,
				file:       path,
				expression: strings.TrimSpace(rule.Expression),
			}
			for _, test := range rule.Tests {
				parsed.tests = append(parsed.tests, ruleTest{
					description: test.Description,
					eventType:   test.Data.Type,
					values:      test.Data.Values,
					expected:    test.Expected,
				})
			}
			set.rules = append(set.rules, parsed)
		}
		return nil
	})
	if err != nil {
		return policySet{}, err
	}

	sort.Slice(set.rules, func(i, j int) bool { return set.rules[i].name < set.rules[j].name })
	return set, nil
}

// readYAML decodes a policy file, tolerating what the backend tolerates.
//
// A yaml.TypeError is reported *after* the document has been decoded — one of the real
// rule files declares `tags` twice — so the fields that did decode are still there, and
// refusing the file would drop a rule that production loads.
func readYAML(path string, into any) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var typeError *yaml.TypeError
	if err := yaml.Unmarshal(content, into); err != nil && !errors.As(err, &typeError) {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// stage is how far a rule got.
type stage int

const (
	stageTranslate stage = iota
	stageCheck
	stagePlan
	stageEvaluate
	stageDone
)

func (s stage) String() string {
	switch s {
	case stageTranslate:
		return "translate"
	case stageCheck:
		return "type-check"
	case stagePlan:
		return "plan"
	case stageEvaluate:
		return "evaluate"
	}
	return "done"
}

// ruleResult is what one rule reached, and how its own test cases went.
type ruleResult struct {
	rule    policyRule
	stopped stage
	err     error

	cases       int
	agreed      int
	disagreed   []string
	bothWrong   []string
	unevaluable []string
}

// measure runs the whole set through both engines.
func measure(set policySet) ([]ruleResult, []seclcel.MacroFailure, error) {
	env, macroFailures, err := seclcel.NewPolicyEnv(seclcel.Policy{Macros: set.macros})
	if err != nil {
		return nil, nil, err
	}

	seclOpts, err := seclMacros(set.macros)
	if err != nil {
		return nil, nil, err
	}

	results := make([]ruleResult, 0, len(set.rules))
	for _, rule := range set.rules {
		results = append(results, measureRule(env, seclOpts, rule))
	}
	return results, macroFailures, nil
}

// seclMacros builds the other engine's macro store, so that both sides see the same
// definitions. A macro SECL itself cannot compile is skipped: the rules using it
// then fail on both sides, which is what the report should show.
func seclMacros(macros []seclcel.Macro) (*eval.Opts, error) {
	var m model.Model

	store := &eval.MacroStore{}
	opts := (&eval.Opts{}).
		WithConstants(model.SECLConstants()).
		WithLegacyFields(model.SECLLegacyFields).
		WithMacroStore(store)

	parsingContext := ast.NewParsingContext(false)
	// One pass is enough for the agent's own macros, none of which refers to another.
	for _, macro := range macros {
		compiled, err := eval.NewMacro(macro.ID, macro.Expression, &m, parsingContext, opts)
		if err != nil {
			continue
		}
		store.Add(compiled)
	}
	return opts, nil
}

func measureRule(env *cel.Env, seclOpts *eval.Opts, rule policyRule) ruleResult {
	result := ruleResult{rule: rule, stopped: stageDone}

	if _, err := seclcel.TranslateWithTypes(rule.expression, seclcel.ModelFieldTypes{}); err != nil {
		result.stopped, result.err = stageTranslate, err
		return result
	}

	if _, err := seclcel.CompileWithTypes(env, rule.expression, seclcel.ModelFieldTypes{}); err != nil {
		result.stopped, result.err = stageCheck, err
		return result
	}

	// NewRule is the whole front end in one call: it checks, resolves the field reads
	// and plans, which is the point where a pattern is compiled.
	planned, err := seclcel.NewRule(env, rule.expression, seclcel.ModelFieldTypes{})
	if err != nil {
		result.stopped, result.err = stagePlan, err
		return result
	}

	// The other engine, over the same expression and the same macros.
	var m model.Model
	seclRule, err := eval.NewRule(rule.name, rule.expression, ast.NewParsingContext(false), seclOpts)
	if err == nil {
		err = seclRule.GenEvaluator(&m)
	}
	if err != nil {
		// SECL refuses it, so there is nothing to compare against. It is not a gap in
		// the translation, and the report counts it apart.
		result.unevaluable = append(result.unevaluable, "SECL: "+firstLine(err))
		return result
	}

	for _, test := range rule.tests {
		result.cases++

		event, err := buildEvent(test)
		if err != nil {
			result.unevaluable = append(result.unevaluable, test.description+": "+firstLine(err))
			continue
		}

		celVerdict, celErr := evalCEL(planned, event)
		seclVerdict, seclErr := evalSECL(seclRule, event)

		if seclErr != nil {
			// The other engine cannot answer either, so the case says nothing about the
			// translation. It is almost always the event: a case that sets no field of
			// the process it names leaves a nil in the model, and both engines read it
			// the same way — the generated CEL readers are the accessors.
			result.unevaluable = append(result.unevaluable,
				fmt.Sprintf("%s: SECL: %s", test.description, firstLine(seclErr)))
			continue
		}
		if celErr != nil {
			result.stopped, result.err = stageEvaluate, celErr
			continue
		}

		switch {
		case celVerdict == seclVerdict && celVerdict == test.expected:
			result.agreed++
		case celVerdict == seclVerdict:
			// Both engines answer the same thing and it is not what the rule expected,
			// which says something about the test case rather than about either engine:
			// the case may leave a field unset that the expression needs.
			result.bothWrong = append(result.bothWrong,
				fmt.Sprintf("%s: both %v, expected %v", test.description, celVerdict, test.expected))
		default:
			result.disagreed = append(result.disagreed,
				fmt.Sprintf("%s: secl=%v cel=%v expected=%v", test.description, seclVerdict, celVerdict, test.expected))
		}
	}

	return result
}

// evalCEL and evalSECL evaluate one rule against one event, containing a panic.
//
// A generated reader dereferences the model the way the accessors do, so a case that
// leaves part of the event unset can take either engine down. Containing it is what lets
// one bad case be reported rather than end the measurement.
func evalCEL(rule *seclcel.Rule, event *model.Event) (verdict bool, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("panicked: %v", recovered)
		}
	}()
	return rule.Eval(seclcel.NewActivation(eval.NewContext(event)))
}

func evalSECL(rule *eval.Rule, event *model.Event) (verdict bool, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("panicked: %v", recovered)
		}
	}()
	return rule.Eval(eval.NewContext(event)), nil
}

// buildEvent turns a declared test case into an event, the way the rule engine's own
// approver tests do: an event type and one SetFieldValue per value.
func buildEvent(test ruleTest) (*model.Event, error) {
	eventType, err := model.ParseEvalEventType(test.eventType)
	if err != nil {
		return nil, err
	}

	event := model.NewFakeEvent()
	event.Type = uint32(eventType)

	for _, field := range sortedValueFields(test.values) {
		if err := event.SetFieldValue(field, fieldValue(test.values[field])); err != nil {
			return nil, fmt.Errorf("setting %s: %w", field, err)
		}
	}
	return event, nil
}

// fieldValue converts what YAML decoded into what the model's setters take: a list
// arrives as []any, and SetFieldValue type-switches on []string and []int.
func fieldValue(value any) any {
	list, ok := value.([]any)
	if !ok {
		return value
	}

	strings := make([]string, 0, len(list))
	ints := make([]int, 0, len(list))
	for _, element := range list {
		switch element := element.(type) {
		case string:
			strings = append(strings, element)
		case int:
			ints = append(ints, element)
		}
	}
	if len(strings) == len(list) {
		return strings
	}
	if len(ints) == len(list) {
		return ints
	}
	return value
}

func sortedValueFields(values map[string]any) []string {
	fields := make([]string, 0, len(values))
	for field := range values {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	return fields
}

// reportCoverage prints the table the plan is steered by, and returns whether
// everything that could be compared agreed.
func reportCoverage(set policySet, results []ruleResult, macroFailures []seclcel.MacroFailure, verbose bool) bool {
	byStage := map[stage][]ruleResult{}
	var cases, agreed int
	var disagreements, bothWrong, unevaluable []string

	for _, result := range results {
		byStage[result.stopped] = append(byStage[result.stopped], result)
		cases += result.cases
		agreed += result.agreed

		for _, entry := range result.disagreed {
			disagreements = append(disagreements, result.rule.name+" — "+entry)
		}
		for _, entry := range result.bothWrong {
			bothWrong = append(bothWrong, result.rule.name+" — "+entry)
		}
		for _, entry := range result.unevaluable {
			unevaluable = append(unevaluable, result.rule.name+" — "+entry)
		}
	}

	fmt.Printf("%d rules, %d macros\n\n", len(set.rules), len(set.macros))

	fmt.Printf("  macros declared     %4d\n", len(set.macros)-len(macroFailures))
	if len(macroFailures) > 0 {
		fmt.Printf("  macros unsupported  %4d\n", len(macroFailures))
		for _, failure := range macroFailures {
			fmt.Printf("      %s\n", firstLine(failure.Err))
		}
	}

	fmt.Printf("\n  rules planned       %4d of %d (%.1f%%)\n",
		len(byStage[stageDone])+len(byStage[stageEvaluate]), len(set.rules),
		100*float64(len(byStage[stageDone])+len(byStage[stageEvaluate]))/float64(len(set.rules)))

	for _, stopped := range []stage{stageTranslate, stageCheck, stagePlan, stageEvaluate} {
		failed := byStage[stopped]
		if len(failed) == 0 {
			continue
		}
		fmt.Printf("  stopped at %-10s %4d\n", stopped, len(failed))
		for _, result := range failed {
			fmt.Printf("      %s: %s\n", result.rule.name, firstLine(result.err))
		}
	}

	fmt.Printf("\n  declared cases      %4d in %d rules\n", cases, countRulesWithCases(results))
	fmt.Printf("  both engines agree  %4d\n", agreed)
	fmt.Printf("  disagree            %4d\n", len(disagreements))
	for _, entry := range disagreements {
		fmt.Printf("      %s\n", entry)
	}
	fmt.Printf("  neither expected    %4d\n", len(bothWrong))
	if verbose {
		for _, entry := range bothWrong {
			fmt.Printf("      %s\n", entry)
		}
	}
	fmt.Printf("  not comparable      %4d\n", len(unevaluable))
	if verbose {
		for _, entry := range unevaluable {
			fmt.Printf("      %s\n", entry)
		}
	}

	return len(disagreements) == 0 && len(byStage[stageEvaluate]) == 0
}

func countRulesWithCases(results []ruleResult) int {
	var n int
	for _, result := range results {
		if result.cases > 0 {
			n++
		}
	}
	return n
}

func firstLine(err error) string {
	if err == nil {
		return ""
	}
	return strings.SplitN(err.Error(), "\n", 2)[0]
}
