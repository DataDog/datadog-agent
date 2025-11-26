// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

// Package runtime holds runtime related files
package runtime

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	secretsnoopfx "github.com/DataDog/datadog-agent/comp/core/secrets/fx-noop"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	secagent "github.com/DataDog/datadog-agent/pkg/security/agent"
	"github.com/DataDog/datadog-agent/pkg/security/clihelpers"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api/transform"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type checkPoliciesCliParams struct {
	*command.GlobalParams

	dir                      string
	evaluateAllPolicySources bool
	windowsModel             bool
}

func commonPolicyCommands(globalParams *command.GlobalParams) []*cobra.Command {
	commonPolicyCmd := &cobra.Command{
		Use:   "policy",
		Short: "Policy related commands",
	}

	commonPolicyCmd.AddCommand(evalCommands(globalParams)...)
	commonPolicyCmd.AddCommand(commonCheckPoliciesCommands(globalParams)...)
	commonPolicyCmd.AddCommand(commonReloadPoliciesCommands(globalParams)...)

	return []*cobra.Command{commonPolicyCmd}
}

type evalCliParams struct {
	*command.GlobalParams

	dir          string
	ruleID       string
	eventFile    string
	debug        bool
	windowsModel bool
}

func evalCommands(globalParams *command.GlobalParams) []*cobra.Command {
	evalArgs := &evalCliParams{
		GlobalParams: globalParams,
	}

	evalCmd := &cobra.Command{
		Use:   "eval",
		Short: "Evaluate given event data against the give rule",
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(evalRule,
				fx.Supply(evalArgs),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParams(""),
					LogParams:    log.ForOneShot(command.LoggerName, "off", false)}),
				core.Bundle(),
				secretsnoopfx.Module(),
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

	return []*cobra.Command{evalCmd}
}

func commonCheckPoliciesCommands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &checkPoliciesCliParams{
		GlobalParams: globalParams,
	}

	commonCheckPoliciesCmd := &cobra.Command{
		Use:   "check",
		Short: "Check policies and return a report",
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(checkPolicies,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParams(""),
					LogParams:    log.ForOneShot(command.LoggerName, "off", false)}),
				core.Bundle(),
				secretsnoopfx.Module(),
			)
		},
	}

	commonCheckPoliciesCmd.Flags().StringVar(&cliParams.dir, "policies-dir", pkgconfigsetup.DefaultRuntimePoliciesDir, "Path to policies directory")
	commonCheckPoliciesCmd.Flags().BoolVar(&cliParams.evaluateAllPolicySources, "loaded-policies", false, "Evaluate loaded policies")
	if runtime.GOOS == "linux" {
		commonCheckPoliciesCmd.Flags().BoolVar(&cliParams.windowsModel, "windows-model", false, "Evaluate policies using the Windows model")
	}

	return []*cobra.Command{commonCheckPoliciesCmd}
}

func commonReloadPoliciesCommands(_ *command.GlobalParams) []*cobra.Command {
	commonReloadPoliciesCmd := &cobra.Command{
		Use:   "reload",
		Short: "Reload policies",
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(reloadRuntimePolicies,
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParams(""),
					LogParams:    log.ForOneShot(command.LoggerName, "info", true)}),
				core.Bundle(),
				secretsnoopfx.Module(),
			)
		},
	}
	return []*cobra.Command{commonReloadPoliciesCmd}
}

// nolint: deadcode, unused
func selfTestCommands(_ *command.GlobalParams) []*cobra.Command {
	selfTestCmd := &cobra.Command{
		Use:   "self-test",
		Short: "Run runtime self test",
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(runRuntimeSelfTest,
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParams(""),
					LogParams:    log.ForOneShot(command.LoggerName, "info", true)}),
				core.Bundle(),
				secretsnoopfx.Module(),
			)
		},
	}

	return []*cobra.Command{selfTestCmd}
}

//nolint:unused // TODO(SEC) Fix unused linter
type processCacheDumpCliParams struct {
	*command.GlobalParams

	withArgs bool
	format   string
}

//nolint:unused // TODO(SEC) Fix unused linter
func processCacheCommands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &processCacheDumpCliParams{
		GlobalParams: globalParams,
	}

	processCacheDumpCmd := &cobra.Command{
		Use:   "dump",
		Short: "dump the process cache",
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(dumpProcessCache,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParams(""),
					LogParams:    log.ForOneShot(command.LoggerName, "info", true)}),
				core.Bundle(),
				secretsnoopfx.Module(),
			)
		},
	}
	processCacheDumpCmd.Flags().BoolVar(&cliParams.withArgs, "with-args", false, "add process arguments to the dump")
	processCacheDumpCmd.Flags().StringVar(&cliParams.format, "format", "dot", "process cache dump format")

	processCacheCmd := &cobra.Command{
		Use:   "process-cache",
		Short: "process cache",
	}
	processCacheCmd.AddCommand(processCacheDumpCmd)

	return []*cobra.Command{processCacheCmd}
}

//nolint:unused // TODO(SEC) Fix unused linter
type dumpNetworkNamespaceCliParams struct {
	*command.GlobalParams

	snapshotInterfaces bool
}

//nolint:unused // TODO(SEC) Fix unused linter
func networkNamespaceCommands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &dumpNetworkNamespaceCliParams{
		GlobalParams: globalParams,
	}

	dumpNetworkNamespaceCmd := &cobra.Command{
		Use:   "dump",
		Short: "dumps the network namespaces held in cache",
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(dumpNetworkNamespace,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParams(""),
					LogParams:    log.ForOneShot(command.LoggerName, "info", true)}),
				core.Bundle(),
				secretsnoopfx.Module(),
			)
		},
	}
	dumpNetworkNamespaceCmd.Flags().BoolVar(&cliParams.snapshotInterfaces, "snapshot-interfaces", true, "snapshot the interfaces of each network namespace during the dump")

	networkNamespaceCmd := &cobra.Command{
		Use:   "network-namespace",
		Short: "network namespace command",
	}
	networkNamespaceCmd.AddCommand(dumpNetworkNamespaceCmd)

	return []*cobra.Command{networkNamespaceCmd}
}

//nolint:unused // TODO(SEC) Fix unused linter
func discardersCommands(_ *command.GlobalParams) []*cobra.Command {

	dumpDiscardersCmd := &cobra.Command{
		Use:   "dump",
		Short: "dump discarders",
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(dumpDiscarders,
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParams(""),
					LogParams:    log.ForOneShot(command.LoggerName, "info", true)}),
				core.Bundle(),
				secretsnoopfx.Module(),
			)
		},
	}

	discardersCmd := &cobra.Command{
		Use:   "discarders",
		Short: "discarders commands",
	}
	discardersCmd.AddCommand(dumpDiscardersCmd)

	return []*cobra.Command{discardersCmd}
}

// nolint: deadcode, unused
func dumpProcessCache(_ log.Component, _ config.Component, _ secrets.Component, processCacheDumpArgs *processCacheDumpCliParams) error {
	client, err := secagent.NewRuntimeSecurityCmdClient()
	if err != nil {
		return fmt.Errorf("unable to create a runtime security client instance: %w", err)
	}
	defer client.Close()

	filename, err := client.DumpProcessCache(processCacheDumpArgs.withArgs, processCacheDumpArgs.format)
	if err != nil {
		return fmt.Errorf("unable to get a process cache dump: %w", err)
	}

	fmt.Printf("Process dump file: %s\n", filename)

	return nil
}

//nolint:unused // TODO(SEC) Fix unused linter
func dumpNetworkNamespace(_ log.Component, _ config.Component, _ secrets.Component, dumpNetworkNamespaceArgs *dumpNetworkNamespaceCliParams) error {
	client, err := secagent.NewRuntimeSecurityCmdClient()
	if err != nil {
		return fmt.Errorf("unable to create a runtime security client instance: %w", err)
	}
	defer client.Close()

	resp, err := client.DumpNetworkNamespace(dumpNetworkNamespaceArgs.snapshotInterfaces)
	if err != nil {
		return fmt.Errorf("couldn't send network namespace cache dump request: %w", err)
	}

	if len(resp.GetError()) > 0 {
		return fmt.Errorf("couldn't dump network namespaces: %s", resp.GetError())
	}

	fmt.Printf("Network namespace dump: %s\n", resp.GetDumpFilename())
	fmt.Printf("Network namespace dump graph: %s\n", resp.GetGraphFilename())
	return nil
}

func checkPolicies(_ log.Component, _ config.Component, args *checkPoliciesCliParams) error {
	if args.evaluateAllPolicySources {
		if args.windowsModel {
			return errors.New("unable to evaluator loaded policies using the windows model")
		}

		client, err := secagent.NewRuntimeSecurityCmdClient()
		if err != nil {
			return fmt.Errorf("unable to create a runtime security client instance: %w", err)
		}
		defer client.Close()

		return checkPoliciesLoaded(client, os.Stdout)
	}
	return clihelpers.CheckPoliciesLocal(clihelpers.CheckPoliciesLocalParams{
		Dir:                      args.dir,
		EvaluateAllPolicySources: args.evaluateAllPolicySources,
		UseWindowsModel:          args.windowsModel,
	}, os.Stdout)
}

func checkPoliciesLoaded(client secagent.SecurityModuleCmdClientWrapper, writer io.Writer) error {
	output, err := client.GetRuleSetReport()
	if err != nil {
		return fmt.Errorf("unable to send request to system-probe: %w", err)
	}
	if len(output.Error) > 0 {
		return fmt.Errorf("get policies request failed: %s", output.Error)
	}

	// extract and report the filters
	transformedOutput := transform.FromProtoToFilterReport(output.GetRuleSetReportMessage().GetFilters())

	content, _ := json.MarshalIndent(transformedOutput, "", "\t")
	_, err = fmt.Fprintf(writer, "%s\n", string(content))
	if err != nil {
		return fmt.Errorf("unable to write out report: %w", err)
	}

	return nil
}

func evalRule(_ log.Component, _ config.Component, _ secrets.Component, evalArgs *evalCliParams) error {
	return clihelpers.EvalRule(clihelpers.EvalRuleParams{
		Dir:             evalArgs.dir,
		UseWindowsModel: evalArgs.windowsModel,
		RuleID:          evalArgs.ruleID,
		EventFile:       evalArgs.eventFile,
	})
}

// nolint: deadcode, unused
func runRuntimeSelfTest(_ log.Component, _ config.Component, _ secrets.Component) error {
	client, err := secagent.NewRuntimeSecurityCmdClient()
	if err != nil {
		return fmt.Errorf("unable to create a runtime security client instance: %w", err)
	}
	defer client.Close()

	selfTestResult, err := client.RunSelfTest()
	if err != nil {
		return fmt.Errorf("unable to get a process self test: %w", err)
	}

	if selfTestResult.Ok {
		fmt.Printf("Runtime self test: OK\n")
	} else {
		fmt.Printf("Runtime self test: error: %v\n", selfTestResult.Error)
	}
	return nil
}

func reloadRuntimePolicies(_ log.Component, _ config.Component, _ secrets.Component) error {
	client, err := secagent.NewRuntimeSecurityCmdClient()
	if err != nil {
		return fmt.Errorf("unable to create a runtime security client instance: %w", err)
	}
	defer client.Close()

	_, err = client.ReloadPolicies()
	if err != nil {
		return fmt.Errorf("unable to reload policies: %w", err)
	}

	return nil
}

// nolint: deadcode, unused
func dumpDiscarders(_ log.Component, _ config.Component, _ secrets.Component) error {
	runtimeSecurityClient, err := secagent.NewRuntimeSecurityCmdClient()
	if err != nil {
		return fmt.Errorf("unable to create a runtime security client instance: %w", err)
	}
	defer runtimeSecurityClient.Close()

	dumpFilename, dumpErr := runtimeSecurityClient.DumpDiscarders()

	if dumpErr != nil {
		return fmt.Errorf("unable to dump discarders: %w", dumpErr)
	}

	fmt.Printf("Discarder dump file: %s\n", dumpFilename)

	return nil
}
