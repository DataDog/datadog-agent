// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package compliance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/compliance/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	regoast "github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/rego"
	regotypes "github.com/open-policy-agent/opa/types"
)

// ResolveAndEvaluateRegoRule both resolves the inputs of a given rego rule,
// using passed resolver, and evaluates them against the rule's rego program.
func ResolveAndEvaluateRegoRule(ctx context.Context, resolver Resolver, benchmark *Benchmark, rule *Rule) []*CheckEvent {
	wrapErr := func(errReason error) []*CheckEvent {
		return []*CheckEvent{NewCheckError(RegoEvaluator, errReason, "", "", rule, benchmark)}
	}
	wrapSkip := func(skipReason error) []*CheckEvent {
		return []*CheckEvent{NewCheckSkipped(RegoEvaluator, skipReason, "", "", rule, benchmark)}
	}

	if !rule.IsRego() {
		return wrapSkip(fmt.Errorf("given rule is not a rego rule %s", rule.ID))
	}

	resolvedInputs, err := resolver.ResolveInputs(ctx, rule)
	if errors.Is(err, ErrIncompatibleEnvironment) {
		return wrapSkip(fmt.Errorf("skipping input resolution for rule=%s: %s", rule.ID, err))
	}
	if err != nil {
		return wrapErr(fmt.Errorf("input resolution error for rule=%s: %w", rule.ID, err))
	}

	log.Infof("running rego check for rule=%s", rule.ID)
	events, err := EvaluateRegoRule(ctx, resolvedInputs, benchmark, rule)
	if err != nil {
		return wrapErr(fmt.Errorf("rego rule evaluation error for rule=%s: %w", rule.ID, err))
	}
	return events
}

// EvaluateRegoRule evaluates the given rule and resolved inputs map against
// the rule's rego program.
func EvaluateRegoRule(ctx context.Context, resolvedInputs ResolvedInputs, benchmark *Benchmark, rule *Rule) ([]*CheckEvent, error) {
	log.Tracef("building rego modules for rule=%s", rule.ID)
	modules, err := buildRegoModules(benchmark.dirname, rule)
	if err != nil {
		return nil, fmt.Errorf("could not build rego modules: %w", err)
	}

	var options []func(*rego.Rego)
	for name, source := range modules {
		options = append(options, rego.Module(name, source))
	}
	options = append(options, regoBuiltins...)
	options = append(options,
		rego.Query("data.datadog.findings"),
		rego.Metrics(metrics.NewRegoTelemetry()),
		rego.UnsafeBuiltins(map[string]struct{}{
			"http.send":   {},
			"opa.runtime": {},
		}),
		rego.Input(resolvedInputs),
	)

	log.Tracef("starting rego evaluation for rule=%s:%s", benchmark.FrameworkID, rule.ID)
	r := rego.New(options...)
	rSet, err := r.Eval(ctx)
	if err != nil {
		return nil, fmt.Errorf("rego eval: %w", err)
	}
	if len(rSet) == 0 || len(rSet[0].Expressions) == 0 {
		return nil, fmt.Errorf("empty results set")
	}

	results, ok := rSet[0].Expressions[0].Value.([]interface{})
	if !ok {
		return nil, fmt.Errorf("could not cast expression value")
	}

	log.TraceFunc(func() string {
		b, _ := json.MarshalIndent(results, "", "\t")
		return fmt.Sprintf("rego evaluation results for %s:%s:\n%s",
			benchmark.FrameworkID, rule.ID, b)
	})

	// We dedupe events based on the pair (resource_type, resource_id)
	dedupe := make(map[string]struct{})
	events := make([]*CheckEvent, 0, len(results))
	for _, data := range results {
		event := newCheckEventFromRegoResult(data, rule, resolvedInputs, benchmark)
		evtID := event.ResourceType + event.ResourceID
		if _, ok := dedupe[evtID]; !ok {
			dedupe[evtID] = struct{}{}
			events = append(events, event)
		}
	}

	return events, nil
}

func newCheckEventFromRegoResult(data interface{}, rule *Rule, resolvedInputs ResolvedInputs, benchmark *Benchmark) *CheckEvent {
	m, ok := data.(map[string]interface{})
	if !ok || m == nil {
		return NewCheckError(RegoEvaluator, fmt.Errorf("failed to cast event"), "", "", rule, benchmark)
	}
	var result CheckResult
	var errReason error
	status, _ := m["status"].(string)
	switch status {
	case "passed", "pass":
		result = CheckPassed
	case "failing", "fail":
		result = CheckFailed
	case "skipped":
		result = CheckSkipped
	case "err", "error":
		d, _ := m["data"].(map[string]interface{})
		errMsg, _ := d["error"].(string)
		if errMsg == "" {
			errMsg = "unknown"
		}
		errReason = fmt.Errorf(errMsg)
	default:
		errReason = fmt.Errorf("rego result invalid: bad status %q", status)
	}
	eventData, _ := m["data"].(map[string]interface{})
	resourceID, _ := m["resource_id"].(string)
	resourceType, _ := m["resource_type"].(string)
	if errReason != nil {
		return NewCheckError(RegoEvaluator, errReason, resourceID, resourceType, rule, benchmark)
	}
	return NewCheckEvent(RegoEvaluator, result, eventData, resourceID, resourceType, rule, benchmark)
}

func buildRegoModules(rootDir string, rule *Rule) (map[string]string, error) {
	modules := map[string]string{
		"datadog_helpers.rego": regoHelpersSource,
	}
	ruleFilename := fmt.Sprintf("%s.rego", rule.ID)
	ruleCode, err := loadFile(rootDir, ruleFilename)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if len(ruleCode) > 0 {
		modules[ruleFilename] = string(ruleCode)
	}
	for _, name := range rule.Imports {
		if _, ok := modules[name]; ok {
			continue
		}
		source, err := loadFile(rootDir, name)
		if err != nil {
			return nil, err
		}
		modules[name] = string(source)
	}
	return modules, nil
}

const regoHelpersSource = `package datadog

raw_finding(status, resource_type, resource_id, event_data) = f {
	f := {
		"status": status,
		"resource_type": resource_type,
		"resource_id": resource_id,
		"data": event_data,
	}
}

passed_finding(resource_type, resource_id, event_data) = f {
	f := raw_finding("passed", resource_type, resource_id, event_data)
}

failing_finding(resource_type, resource_id, event_data) = f {
	f := raw_finding("failing", resource_type, resource_id, event_data)
}

skipped_finding(resource_type, resource_id, error_msg) = f {
	f := raw_finding("skipped", resource_type, resource_id, {
		"error": error_msg
	})
}

error_finding(resource_type, resource_id, error_msg) = f {
	f := raw_finding("error", resource_type, resource_id, {
		"error": error_msg
	})
}
`

var regoBuiltins = []func(*rego.Rego){
	rego.Function1(
		&rego.Function{
			Name: "parse_octal",
			Decl: regotypes.NewFunction(regotypes.Args(regotypes.S), regotypes.N),
		},
		func(_ rego.BuiltinContext, a *regoast.Term) (*regoast.Term, error) {
			str, ok := a.Value.(regoast.String)
			if !ok {
				return nil, fmt.Errorf("rego builtin parse_octal was not given a String")
			}
			value, err := strconv.ParseInt(string(str), 8, 0)
			if err != nil {
				return nil, fmt.Errorf("rego builtin parse_octal failed to parse into int: %w", err)
			}
			return regoast.IntNumberTerm(int(value)), err
		},
	),
}
