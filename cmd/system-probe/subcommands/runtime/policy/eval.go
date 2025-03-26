// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

// Package policy holds policy CLI subcommand related files
package policy

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"

	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	secconfig "github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/probe/kfilters"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	winmodel "github.com/DataDog/datadog-agent/pkg/security/seclwin/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/spf13/cobra"
	"go.uber.org/fx"
)

type evalCliParams struct {
	*command.GlobalParams

	dir          string
	ruleID       string
	eventFile    string
	debug        bool
	windowsModel bool
}

// EvalCommand returns the CLI command for "policy eval"
func EvalCommand(globalParams *command.GlobalParams) *cobra.Command {
	evalArgs := &evalCliParams{
		GlobalParams: globalParams,
	}

	evalCmd := &cobra.Command{
		Use:   "eval",
		Short: "Evaluate given event data against the give rule",
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(EvaluateRule,
				fx.Supply(evalArgs),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParams("", config.WithConfigMissingOK(true)),
					SecretParams: secrets.NewDisabledParams(),
					LogParams:    log.ForOneShot("SYS-PROBE", "info", true)}),
				core.Bundle(),
			)
		},
	}

	evalCmd.Flags().StringVar(&evalArgs.dir, "policies-dir", pkgconfigsetup.DefaultRuntimePoliciesDir, "Path to policies directory")
	evalCmd.Flags().StringVar(&evalArgs.ruleID, "rule-id", "", "Rule ID to evaluate")
	_ = evalCmd.MarkFlagRequired("rule-id")
	evalCmd.Flags().StringVar(&evalArgs.eventFile, "event-file", "", "File of the event data")
	_ = evalCmd.MarkFlagRequired("event-file")
	evalCmd.Flags().BoolVar(&evalArgs.debug, "debug", false, "Display an event dump if the evaluation fail")
	if runtime.GOOS == "linux" {
		evalCmd.Flags().BoolVar(&evalArgs.windowsModel, "windows-model", false, "Use the Windows model")
	}

	return evalCmd
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

// EvalReport defines a report of an evaluation
type EvalReport struct {
	Succeeded bool
	Approvers map[string]rules.Approvers
	Event     eval.Event
	Error     error `json:",omitempty"`
}

// EventData defines the structure used to represent an event
type EventData struct {
	Type   eval.EventType
	Values map[string]interface{}
}

func eventDataFromJSON(file string) (eval.Event, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	decoder := json.NewDecoder(f)
	decoder.UseNumber()

	var eventData EventData
	if err := decoder.Decode(&eventData); err != nil {
		return nil, err
	}

	kind := secconfig.ParseEvalEventType(eventData.Type)
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

	for k, v := range eventData.Values {
		switch v := v.(type) {
		case json.Number:
			value, err := v.Int64()
			if err != nil {
				return nil, err
			}
			if err := event.SetFieldValue(k, int(value)); err != nil {
				return nil, err
			}
		default:
			if err := event.SetFieldValue(k, v); err != nil {
				return nil, err
			}
		}
	}

	return event, nil
}

func newAgentVersionFilter() (*rules.AgentVersionFilter, error) {
	agentVersion, err := utils.GetAgentSemverVersion()
	if err != nil {
		return nil, err
	}

	return rules.NewAgentVersionFilter(agentVersion)
}

// EvaluateRule evaluates a rule
func EvaluateRule(_ log.Component, _ config.Component, _ secrets.Component, evalArgs *evalCliParams) error {
	policiesDir := evalArgs.dir

	// enabled all the rules
	enabled := map[eval.EventType]bool{"*": true}

	ruleOpts := rules.NewRuleOpts(enabled)
	evalOpts := newEvalOpts(evalArgs.windowsModel)
	ruleOpts.WithLogger(seclog.DefaultLogger)

	agentVersionFilter, err := newAgentVersionFilter()
	if err != nil {
		return fmt.Errorf("failed to create agent version filter: %w", err)
	}

	loaderOpts := rules.PolicyLoaderOpts{
		MacroFilters: []rules.MacroFilter{
			agentVersionFilter,
		},
		RuleFilters: []rules.RuleFilter{
			&rules.RuleIDFilter{
				ID: evalArgs.ruleID,
			},
		},
	}

	provider, err := rules.NewPoliciesDirProvider(policiesDir)
	if err != nil {
		return err
	}

	loader := rules.NewPolicyLoader(provider)

	var ruleSet *rules.RuleSet
	if evalArgs.windowsModel {
		ruleSet = rules.NewRuleSet(&winmodel.Model{}, newFakeWindowsEvent, ruleOpts, evalOpts)
	} else {
		ruleSet = rules.NewRuleSet(&model.Model{}, newFakeEvent, ruleOpts, evalOpts)
	}

	if err := ruleSet.LoadPolicies(loader, loaderOpts); err.ErrorOrNil() != nil {
		return err
	}

	event, err := eventDataFromJSON(evalArgs.eventFile)
	if err != nil {
		return err
	}

	report := EvalReport{
		Event: event,
	}

	if !evalArgs.windowsModel {
		approvers, err := ruleSet.GetApprovers(kfilters.GetCapababilities())
		if err != nil {
			report.Error = err
		} else {
			report.Approvers = approvers
		}
	}

	report.Succeeded = ruleSet.Evaluate(event)
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
