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
	"io"
	"os"
	"runtime"

	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	secagent "github.com/DataDog/datadog-agent/pkg/security/agent"
	pconfig "github.com/DataDog/datadog-agent/pkg/security/probe/config"
	"github.com/DataDog/datadog-agent/pkg/security/probe/kfilters"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	winmodel "github.com/DataDog/datadog-agent/pkg/security/seclwin/model"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/spf13/cobra"
	"go.uber.org/fx"
)

type checkPoliciesCliParams struct {
	*command.GlobalParams

	dir                      string
	evaluateAllPolicySources bool
	windowsModel             bool
}

// CheckPoliciesCommand returns the CLI command for "policy check"
func CheckPoliciesCommand(globalParams *command.GlobalParams) *cobra.Command {
	cliParams := &checkPoliciesCliParams{
		GlobalParams: globalParams,
	}

	checkPoliciesCmd := &cobra.Command{
		Use:   "check",
		Short: "Check policies and return a report",
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(CheckPolicies,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParams("", config.WithConfigMissingOK(true)),
					SecretParams: secrets.NewDisabledParams(),
					LogParams:    log.ForOneShot("SYS-PROBE", "off", false)}),
				core.Bundle(),
			)
		},
	}

	checkPoliciesCmd.Flags().StringVar(&cliParams.dir, "policies-dir", pkgconfigsetup.DefaultRuntimePoliciesDir, "Path to policies directory")
	checkPoliciesCmd.Flags().BoolVar(&cliParams.evaluateAllPolicySources, "loaded-policies", false, "Evaluate loaded policies")
	if runtime.GOOS == "linux" {
		checkPoliciesCmd.Flags().BoolVar(&cliParams.windowsModel, "windows-model", false, "Evaluate policies using the Windows model")
	}

	return checkPoliciesCmd
}

// CheckPolicies checks the policies
func CheckPolicies(_ log.Component, _ config.Component, args *checkPoliciesCliParams) error {
	if args.evaluateAllPolicySources {
		if args.windowsModel {
			return errors.New("unable to evaluator loaded policies using the windows model")
		}

		client, err := secagent.NewRuntimeSecurityClient()
		if err != nil {
			return fmt.Errorf("unable to create a runtime security client instance: %w", err)
		}
		defer client.Close()

		return checkPoliciesLoaded(client, os.Stdout)
	}
	return checkPoliciesLocal(args, os.Stdout)
}

func checkPoliciesLocal(args *checkPoliciesCliParams, writer io.Writer) error {
	cfg := &pconfig.Config{
		EnableKernelFilters: true,
		EnableApprovers:     true,
		EnableDiscarders:    true,
		PIDCacheSize:        1,
	}

	// enabled all the rules
	enabled := map[eval.EventType]bool{"*": true}

	ruleOpts := rules.NewRuleOpts(enabled)
	evalOpts := newEvalOpts(args.windowsModel)

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
			agentVersionFilter,
		},
	}

	provider, err := rules.NewPoliciesDirProvider(args.dir)
	if err != nil {
		return err
	}

	loader := rules.NewPolicyLoader(provider)

	var ruleSet *rules.RuleSet
	if args.windowsModel {
		ruleSet = rules.NewRuleSet(&winmodel.Model{}, newFakeWindowsEvent, ruleOpts, evalOpts)
		ruleSet.SetFakeEventCtor(newFakeWindowsEvent)
	} else {
		ruleSet = rules.NewRuleSet(&model.Model{}, newFakeEvent, ruleOpts, evalOpts)
		ruleSet.SetFakeEventCtor(newFakeEvent)
	}
	if err := ruleSet.LoadPolicies(loader, loaderOpts); err.ErrorOrNil() != nil {
		return err
	}

	report, err := kfilters.NewApplyRuleSetReport(cfg, ruleSet)
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

func checkPoliciesLoaded(client secagent.SecurityModuleClientWrapper, writer io.Writer) error {
	output, err := client.GetRuleSetReport()
	if err != nil {
		return fmt.Errorf("unable to send request to system-probe: %w", err)
	}
	if len(output.Error) > 0 {
		return fmt.Errorf("get policies request failed: %s", output.Error)
	}

	transformedOutput := output.GetRuleSetReportMessage().FromProtoToKFiltersRuleSetReport()

	content, _ := json.MarshalIndent(transformedOutput, "", "\t")
	_, err = fmt.Fprintf(writer, "%s\n", string(content))
	if err != nil {
		return fmt.Errorf("unable to write out report: %w", err)
	}

	return nil
}
