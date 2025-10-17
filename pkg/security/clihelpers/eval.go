// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

// Package clihelpers holds common CLI helpers
package clihelpers

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"

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

	// enabled all the rules
	enabled := map[eval.EventType]bool{"*": true}

	ruleOpts := rules.NewRuleOpts(enabled)
	evalOpts := newEvalOpts(evalArgs.UseWindowsModel)
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

	ctx := eval.NewContext(event)

	for varName, value := range variables {
		definition, ok := evalOpts.VariableStore.GetDefinition(eval.VariableName(varName))
		if definition == nil || !ok {
			var definedVariableNames []string
			evalOpts.VariableStore.IterVariableDefinitions(func(definition eval.VariableDefinition) {
				definedVariableNames = append(definedVariableNames, definition.VariableName(true))
			})
			return report, fmt.Errorf("no variable named `%s` was found in current ruleset (found: %q)", varName, definedVariableNames)
		}
		instance, added, err := definition.AddNewInstance(ctx)
		if err != nil {
			return report, fmt.Errorf("failed to register new variable instance `%s`: %s", varName, err)
		}
		if !added {
			return report, fmt.Errorf("failed to add new variable intsance `%s`", varName)
		}
		err = instance.Set(value)
		if err != nil {
			return report, fmt.Errorf("failed to set value of variable `%s` to value `%v`: %s", varName, value, err)
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
	kind, err := model.ParseEvalEventType(testData.Type)
	if err != nil {
		return nil, err
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

func variablesFromTestData(testData TestData) (map[string]any, error) {
	variables := make(map[string]any)

	for varName, value := range testData.Variables {
		switch value := value.(type) {
		case string:
			variables[varName] = value
		case json.Number:
			v, err := value.Int64()
			if err != nil {
				return nil, fmt.Errorf("failed to convert %s to int: %w", varName, err)
			}
			variables[varName] = int(v)
		case float64:
			variables[varName] = int(value)
		case bool:
			variables[varName] = value
		case []any:
			if len(value) == 0 {
				return nil, fmt.Errorf("test variable `%s` has unknown type: %T", varName, value)
			}
			switch value[0].(type) {
			case string:
				values := make([]string, 0, len(value))
				for _, v := range value {
					values = append(values, v.(string))
				}
				variables[varName] = values
			case json.Number:
				values := make([]int, 0, len(value))
				for _, v := range value {
					v, err := v.(json.Number).Int64()
					if err != nil {
						return nil, fmt.Errorf("failed to convert %s to int: %w", varName, err)
					}
					values = append(values, int(v))
				}
				variables[varName] = values
			case float64:
				values := make([]int, 0, len(value))
				for _, v := range value {
					values = append(values, int(v.(float64)))
				}
				variables[varName] = values
			default:
				return nil, fmt.Errorf("test variable `%s` has unknown type: %T", varName, value)
			}
		default:
			return nil, fmt.Errorf("test variable `%s` has unknown type: %T", varName, value)
		}
	}

	return variables, nil
}

func dataFromJSON(decoder *json.Decoder) (eval.Event, map[string]any, error) {
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
