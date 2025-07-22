// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

// Package clihelpers holds common CLI helpers
package clihelpers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"

	secconfig "github.com/DataDog/datadog-agent/pkg/security/config"
	pconfig "github.com/DataDog/datadog-agent/pkg/security/probe/config"
	"github.com/DataDog/datadog-agent/pkg/security/probe/kfilters"
	"github.com/DataDog/datadog-agent/pkg/security/rules/filtermodel"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	winmodel "github.com/DataDog/datadog-agent/pkg/security/seclwin/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// EvalReport defines a report of an evaluation
type EvalReport struct {
	Succeeded bool
	Approvers map[string]rules.Approvers
	Event     eval.Event
	Error     error `json:",omitempty"`
}

// TestData defines the structure used to represent an event and its variables
type TestData struct {
	Type      eval.EventType
	Values    map[string]any
	Variables map[string]any
}

// EvalRuleParams are parameters to the EvalRule function
type EvalRuleParams struct {
	Dir             string
	UseWindowsModel bool
	RuleID          string
	EventFile       string
}

func evalRule(provider rules.PolicyProvider, decoder *json.Decoder, evalArgs EvalRuleParams) (EvalReport, error) {
	var report EvalReport

	event, variables, err := dataFromJSON(decoder)
	if err != nil {
		return report, err
	}

	report.Event = event

	// store the variables values so that we can reapply them after policies are loaded
	variablesValues := make(map[string]any)
	for k, v := range variables {
		rv, ok := v.(eval.Variable)
		if !ok {
			continue
		}

		value, _ := rv.GetValue()
		variablesValues[k] = value
	}

	// enabled all the rules
	enabled := map[eval.EventType]bool{"*": true}

	ruleOpts := rules.NewRuleOpts(enabled)
	evalOpts := newEvalOpts(evalArgs.UseWindowsModel).WithVariables(variables)
	ruleOpts.WithLogger(seclog.DefaultLogger)

	agentVersionFilter, err := newAgentVersionFilter()
	if err != nil {
		return report, fmt.Errorf("failed to create agent version filter: %w", err)
	}

	loaderOpts := rules.PolicyLoaderOpts{
		MacroFilters: []rules.MacroFilter{
			agentVersionFilter,
		},
		RuleFilters: []rules.RuleFilter{
			&rules.RuleIDFilter{
				ID: evalArgs.RuleID,
			},
		},
	}

	loader := rules.NewPolicyLoader(provider)

	var ruleSet *rules.RuleSet
	if evalArgs.UseWindowsModel {
		ruleSet = rules.NewRuleSet(&winmodel.Model{}, newFakeWindowsEvent, ruleOpts, evalOpts)
	} else {
		ruleSet = rules.NewRuleSet(&model.Model{}, newFakeEvent, ruleOpts, evalOpts)
	}

	if _, err := ruleSet.LoadPolicies(loader, loaderOpts); err.ErrorOrNil() != nil {
		return report, err
	}

	// reapply the variables values
	vars := ruleSet.GetVariables()
	for k, v := range variablesValues {
		if _, ok := vars[k]; ok {
			if mv, ok := vars[k].(eval.MutableVariable); ok {
				if err := mv.Set(nil, v); err != nil {
					return report, fmt.Errorf("failed to set variable %s: %w", k, err)
				}
			}
		}
	}

	if !evalArgs.UseWindowsModel {
		approvers, _, _, err := ruleSet.GetApprovers(kfilters.GetCapababilities())
		if err != nil {
			report.Error = err
		} else {
			report.Approvers = approvers
		}
	}

	report.Succeeded = ruleSet.Evaluate(event)

	return report, nil
}

// EvalRule evaluates a rule against an event
func EvalRule(evalArgs EvalRuleParams) error {
	policiesDir := evalArgs.Dir

	f, err := os.Open(evalArgs.EventFile)
	if err != nil {
		return err
	}
	defer f.Close()

	decoder := json.NewDecoder(f)
	decoder.UseNumber()

	provider, err := rules.NewPoliciesDirProvider(policiesDir)
	if err != nil {
		return err
	}

	report, err := evalRule(provider, decoder, evalArgs)
	if err != nil {
		return err
	}

	output, err := json.MarshalIndent(report, "", "    ")
	if err != nil {
		return err
	}
	fmt.Printf("%s\n", string(output))

	if !report.Succeeded {
		os.Exit(-1)
	}

	return nil
}

func eventFromTestData(testData TestData) (eval.Event, error) {

	kind := secconfig.ParseEvalEventType(testData.Type)
	if kind == model.UnknownEventType {
		return nil, errors.New("unknown event type")
	}

	event := &model.Event{
		BaseEvent: model.BaseEvent{
			Type:             uint32(kind),
			FieldHandlers:    &model.FakeFieldHandlers{},
			ContainerContext: &model.ContainerContext{},
		},
	}
	event.Init()

	for k, v := range testData.Values {
		switch v := v.(type) {
		case json.Number:
			value, err := v.Int64()
			if err != nil {
				return nil, err
			}
			if err := event.SetFieldValue(k, int(value)); err != nil {
				return nil, err
			}
		case []any:
			if stringSlice, ok := anySliceToStringSlice(v); ok {
				if err := event.SetFieldValue(k, stringSlice); err != nil {
					return nil, err
				}
			} else {
				if err := event.SetFieldValue(k, v); err != nil {
					return nil, err
				}
			}
		default:
			if err := event.SetFieldValue(k, v); err != nil {
				return nil, err
			}
		}
	}

	return event, nil
}

func variablesFromTestData(testData TestData) (map[string]eval.SECLVariable, error) {
	variables := make(map[string]eval.SECLVariable)

	// copy the embedded variables
	for k, v := range model.SECLVariables {
		variables[k] = v
	}

	varOpts := eval.VariableOpts{
		TTL: 10000000,
	}

	// add the variables from the test data
	for k, v := range testData.Variables {
		switch v := v.(type) {
		case string:
			if rules.IsScopeVariable(k) {
				variables[k] = eval.NewScopedStringVariable(func(_ *eval.Context) (string, bool) {
					return v, true
				}, nil)
			} else {
				variables[k] = eval.NewStringVariable(v, varOpts)
			}
		case []any:
			switch v[0].(type) {
			case string:
				values := make([]string, len(v))
				for i, value := range v {
					values[i] = value.(string)
				}
				if rules.IsScopeVariable(k) {
					variables[k] = eval.NewScopedStringArrayVariable(func(_ *eval.Context) ([]string, bool) {
						return values, true
					}, nil)
				} else {
					variables[k] = eval.NewStringArrayVariable(values, varOpts)
				}
			case json.Number:
				values := make([]int, len(v))
				for i, value := range v {
					v64, err := value.(json.Number).Int64()
					if err != nil {
						return nil, fmt.Errorf("failed to convert %s to int: %w", v, err)
					}
					values[i] = int(v64)
				}

				if rules.IsScopeVariable(k) {
					variables[k] = eval.NewScopedIntArrayVariable(func(_ *eval.Context) ([]int, bool) {
						return values, true
					}, nil)
				} else {
					variables[k] = eval.NewIntArrayVariable(values, varOpts)
				}
			default:
				return nil, fmt.Errorf("unknown variable type %s: %T", k, v)
			}
		case json.Number:
			value, err := v.Int64()
			if err != nil {
				return nil, fmt.Errorf("failed to convert %s to int: %w", v, err)
			}
			if rules.IsScopeVariable(k) {
				variables[k] = eval.NewScopedIntVariable(func(_ *eval.Context) (int, bool) {
					return int(value), true
				}, nil)
			} else {
				variables[k] = eval.NewIntVariable(int(value), varOpts)
			}
		case bool:
			if rules.IsScopeVariable(k) {
				variables[k] = eval.NewScopedBoolVariable(func(_ *eval.Context) (bool, bool) {
					return v, true
				}, nil)
			} else {
				variables[k] = eval.NewBoolVariable(v, varOpts)
			}
		default:
			return nil, fmt.Errorf("unknown variable type %s: %T", k, v)
		}
	}

	return variables, nil
}

func dataFromJSON(decoder *json.Decoder) (eval.Event, map[string]eval.SECLVariable, error) {
	var testData TestData
	if err := decoder.Decode(&testData); err != nil {
		return nil, nil, err
	}

	event, err := eventFromTestData(testData)
	if err != nil {
		return nil, nil, err
	}

	variables, err := variablesFromTestData(testData)
	if err != nil {
		return nil, nil, err
	}

	return event, variables, nil
}

func anySliceToStringSlice(in []any) ([]string, bool) {
	out := make([]string, len(in))
	for i, v := range in {
		val, ok := v.(string)
		if !ok {
			return nil, false
		}
		out[i] = val
	}
	return out, true
}

func newFakeEvent() eval.Event {
	return model.NewFakeEvent()
}

func newFakeWindowsEvent() eval.Event {
	return winmodel.NewFakeEvent()
}

func newEvalOpts(winModel bool) *eval.Opts {
	var evalOpts eval.Opts

	if winModel {
		evalOpts.
			WithConstants(winmodel.SECLConstants()).
			WithLegacyFields(winmodel.SECLLegacyFields).
			WithVariables(model.SECLVariables)
	} else {
		evalOpts.
			WithConstants(model.SECLConstants()).
			WithLegacyFields(model.SECLLegacyFields).
			WithVariables(model.SECLVariables)
	}

	return &evalOpts
}

func newAgentVersionFilter() (*rules.AgentVersionFilter, error) {
	agentVersion, err := utils.GetAgentSemverVersion()
	if err != nil {
		return nil, err
	}

	return rules.NewAgentVersionFilter(agentVersion)
}

// CheckPoliciesLocalParams are parameters to the CheckPoliciesLocal function
type CheckPoliciesLocalParams struct {
	Dir                      string
	EvaluateAllPolicySources bool
	UseWindowsModel          bool
}

// CheckPoliciesLocal checks the policies in a directory
func CheckPoliciesLocal(args CheckPoliciesLocalParams, writer io.Writer) error {
	cfg := &pconfig.Config{
		EnableKernelFilters: true,
		EnableApprovers:     true,
		EnableDiscarders:    true,
		PIDCacheSize:        1,
	}

	// enabled all the rules
	enabled := map[eval.EventType]bool{"*": true}

	ruleOpts := rules.NewRuleOpts(enabled)
	evalOpts := newEvalOpts(args.UseWindowsModel)

	ruleOpts.WithLogger(seclog.DefaultLogger)

	agentVersionFilter, err := newAgentVersionFilter()
	if err != nil {
		return fmt.Errorf("failed to create agent version filter: %w", err)
	}

	os := runtime.GOOS
	if args.UseWindowsModel {
		os = "windows"
	}

	ruleFilterModel := filtermodel.NewOSOnlyFilterModel(os)
	seclRuleFilter := rules.NewSECLRuleFilter(ruleFilterModel)

	loaderOpts := rules.PolicyLoaderOpts{
		MacroFilters: []rules.MacroFilter{
			agentVersionFilter,
			seclRuleFilter,
		},
		RuleFilters: []rules.RuleFilter{
			agentVersionFilter,
			seclRuleFilter,
		},
	}

	provider, err := rules.NewPoliciesDirProvider(args.Dir)
	if err != nil {
		return err
	}

	loader := rules.NewPolicyLoader(provider)

	var ruleSet *rules.RuleSet
	if args.UseWindowsModel {
		ruleSet = rules.NewRuleSet(&winmodel.Model{}, newFakeWindowsEvent, ruleOpts, evalOpts)
		ruleSet.SetFakeEventCtor(newFakeWindowsEvent)
	} else {
		ruleSet = rules.NewRuleSet(&model.Model{}, newFakeEvent, ruleOpts, evalOpts)
		ruleSet.SetFakeEventCtor(newFakeEvent)
	}
	if _, err := ruleSet.LoadPolicies(loader, loaderOpts); err.ErrorOrNil() != nil {
		return err
	}

	report, err := kfilters.ComputeFilters(cfg, ruleSet)
	if err != nil {
		return err
	}

	content, _ := json.MarshalIndent(report, "", "\t")
	_, err = fmt.Fprintf(writer, "%s\n", string(content))
	if err != nil {
		return fmt.Errorf("unable to write out report: %w", err)
	}

	return nil
}
