// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

// Package runtime holds runtime related files
package runtime

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	secagent "github.com/DataDog/datadog-agent/pkg/security/agent"
	secconfig "github.com/DataDog/datadog-agent/pkg/security/config"
	pconfig "github.com/DataDog/datadog-agent/pkg/security/probe/config"
	"github.com/DataDog/datadog-agent/pkg/security/probe/kfilters"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	winmodel "github.com/DataDog/datadog-agent/pkg/security/seclwin/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/version"
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
	commonPolicyCmd.AddCommand(downloadPolicyCommands(globalParams)...)

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
					ConfigParams: config.NewAgentParams("", config.WithConfigMissingOK(true)),
					SecretParams: secrets.NewDisabledParams(),
					LogParams:    log.ForOneShot("SYS-PROBE", "info", true)}),
				core.Bundle(),
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
					ConfigParams: config.NewAgentParams("", config.WithConfigMissingOK(true)),
					SecretParams: secrets.NewDisabledParams(),
					LogParams:    log.ForOneShot("SYS-PROBE", "info", true)}),
				core.Bundle(),
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
					ConfigParams: config.NewAgentParams("", config.WithConfigMissingOK(true)),
					SecretParams: secrets.NewDisabledParams(),
					LogParams:    log.ForOneShot("SYS-PROBE", "info", true)}),
				core.Bundle(),
			)
		},
	}

	return []*cobra.Command{selfTestCmd}
}

type downloadPolicyCliParams struct {
	*command.GlobalParams

	check      bool
	outputPath string
	source     string
}

func downloadPolicyCommands(globalParams *command.GlobalParams) []*cobra.Command {
	downloadPolicyArgs := &downloadPolicyCliParams{
		GlobalParams: globalParams,
	}

	downloadPolicyCmd := &cobra.Command{
		Use:   "download",
		Short: "Download policies",
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(downloadPolicy,
				fx.Supply(downloadPolicyArgs),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParams(globalParams.ConfFilePath),
					SecretParams: secrets.NewDisabledParams(),
					LogParams:    log.ForOneShot("SYS-PROBE", "info", true)}),
				core.Bundle(),
			)
		},
	}

	downloadPolicyCmd.Flags().BoolVar(&downloadPolicyArgs.check, "check", false, "Check policies after downloading")
	downloadPolicyCmd.Flags().StringVar(&downloadPolicyArgs.outputPath, "output-path", "", "Output path for downloaded policies")
	downloadPolicyCmd.Flags().StringVar(&downloadPolicyArgs.source, "source", "all", `Specify wether should download the custom, default or all policies. allowed: "all", "default", "custom"`)

	return []*cobra.Command{downloadPolicyCmd}
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
					ConfigParams: config.NewAgentParams("", config.WithConfigMissingOK(true)),
					SecretParams: secrets.NewDisabledParams(),
					LogParams:    log.ForOneShot("SYS-PROBE", "info", true)}),
				core.Bundle(),
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
					ConfigParams: config.NewAgentParams("", config.WithConfigMissingOK(true)),
					SecretParams: secrets.NewDisabledParams(),
					LogParams:    log.ForOneShot("SYS-PROBE", "info", true)}),
				core.Bundle(),
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
					ConfigParams: config.NewAgentParams("", config.WithConfigMissingOK(true)),
					SecretParams: secrets.NewDisabledParams(),
					LogParams:    log.ForOneShot("SYS-PROBE", "info", true)}),
				core.Bundle(),
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
	client, err := secagent.NewRuntimeSecurityClient()
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

// nolint: deadcode, unused
func dumpNetworkNamespace(_ log.Component, _ config.Component, _ secrets.Component, dumpNetworkNamespaceArgs *dumpNetworkNamespaceCliParams) error {
	client, err := secagent.NewRuntimeSecurityClient()
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

//nolint:unused // TODO(SEC) Fix unused linter
func printStorageRequestMessage(prefix string, storage *api.StorageRequestMessage) {
	fmt.Printf("%so file: %s\n", prefix, storage.GetFile())
	fmt.Printf("%s  format: %s\n", prefix, storage.GetFormat())
	fmt.Printf("%s  storage type: %s\n", prefix, storage.GetType())
	fmt.Printf("%s  compression: %v\n", prefix, storage.GetCompression())
}

func newAgentVersionFilter() (*rules.AgentVersionFilter, error) {
	agentVersion, err := utils.GetAgentSemverVersion()
	if err != nil {
		return nil, err
	}

	return rules.NewAgentVersionFilter(agentVersion)
}

func checkPolicies(_ log.Component, _ config.Component, args *checkPoliciesCliParams) error {
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

	provider, err := rules.NewPoliciesDirProvider(args.dir, false)
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

	m := &model.Model{}
	event := m.NewDefaultEventWithType(kind)
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

func evalRule(_ log.Component, _ config.Component, _ secrets.Component, evalArgs *evalCliParams) error {
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

	provider, err := rules.NewPoliciesDirProvider(policiesDir, false)
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

// nolint: deadcode, unused
func runRuntimeSelfTest(_ log.Component, _ config.Component, _ secrets.Component) error {
	client, err := secagent.NewRuntimeSecurityClient()
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
	client, err := secagent.NewRuntimeSecurityClient()
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

func downloadPolicy(log log.Component, config config.Component, _ secrets.Component, downloadPolicyArgs *downloadPolicyCliParams) error {
	var outputFile *os.File

	apiKey := config.GetString("api_key")
	appKey := config.GetString("app_key")

	if apiKey == "" {
		return errors.New("API key is empty")
	}

	if appKey == "" {
		return errors.New("application key is empty")
	}

	site := config.GetString("site")
	if site == "" {
		site = "datadoghq.com"
	}

	var outputWriter io.Writer
	if downloadPolicyArgs.outputPath == "" || downloadPolicyArgs.outputPath == "-" {
		outputWriter = os.Stdout
	} else {
		f, err := os.Create(downloadPolicyArgs.outputPath)
		if err != nil {
			return err
		}
		defer f.Close()
		outputFile = f
		outputWriter = f
	}

	downloadURL := fmt.Sprintf("https://api.%s/api/v2/remote_config/products/cws/policy/download", site)
	fmt.Fprintf(os.Stderr, "Policy download url: %s\n", downloadURL)

	headers := map[string]string{
		"Content-Type":       "application/json",
		"DD-API-KEY":         apiKey,
		"DD-APPLICATION-KEY": appKey,
	}

	if av, err := version.Agent(); err == nil {
		headers["DD-AGENT-VERSION"] = av.GetNumberAndPre()
	}

	ctx := context.Background()
	res, err := httputils.Get(ctx, downloadURL, headers, 10*time.Second, config)
	if err != nil {
		return err
	}

	// Unzip the downloaded file containing both default and custom policies
	resBytes := []byte(res)
	reader, err := zip.NewReader(bytes.NewReader(resBytes), int64(len(resBytes)))
	if err != nil {
		return err
	}

	var defaultPolicy []byte
	var customPolicies []string

	for _, file := range reader.File {
		if strings.HasSuffix(file.Name, ".policy") {
			pf, err := file.Open()
			if err != nil {
				return err
			}
			policyData, err := io.ReadAll(pf)
			pf.Close()
			if err != nil {
				return err
			}

			if file.Name == "default.policy" {
				defaultPolicy = policyData
			} else {
				customPolicies = append(customPolicies, string(policyData))
			}
		}
	}

	tempDir, err := os.MkdirTemp("", "policy_check")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	if err := os.WriteFile(path.Join(tempDir, "default.policy"), defaultPolicy, 0644); err != nil {
		return err
	}
	for i, customPolicy := range customPolicies {
		if err := os.WriteFile(path.Join(tempDir, fmt.Sprintf("custom%d.policy", i+1)), []byte(customPolicy), 0644); err != nil {
			return err
		}
	}

	if downloadPolicyArgs.check {
		if err := checkPolicies(log, config, &checkPoliciesCliParams{dir: tempDir}); err != nil {
			return err
		}
	}

	// Extract and merge rules from custom policies
	var customRules string
	for _, customPolicy := range customPolicies {
		customPolicyLines := strings.Split(customPolicy, "\n")
		rulesIndex := -1
		for i, line := range customPolicyLines {
			if strings.TrimSpace(line) == "rules:" {
				rulesIndex = i
				break
			}
		}
		if rulesIndex != -1 && rulesIndex+1 < len(customPolicyLines) {
			customRules += "\n" + strings.Join(customPolicyLines[rulesIndex+1:], "\n")
		}
	}

	// Output depending on user's specification
	var outputContent string
	switch downloadPolicyArgs.source {
	case "all":
		outputContent = string(defaultPolicy) + customRules
	case "default":
		outputContent = string(defaultPolicy)
	case "custom":
		outputContent = string(customRules)
	default:
		return errors.New("invalid source specified")
	}

	_, err = outputWriter.Write([]byte(outputContent))
	if err != nil {
		return err
	}

	if outputFile != nil {
		return outputFile.Close()
	}

	return err
}

// nolint: deadcode, unused
func dumpDiscarders(_ log.Component, _ config.Component, _ secrets.Component) error {
	runtimeSecurityClient, err := secagent.NewRuntimeSecurityClient()
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
