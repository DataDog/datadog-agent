// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package runtime holds runtime related files
package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/security-agent/command"
	"github.com/DataDog/datadog-agent/cmd/security-agent/flags"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	secagent "github.com/DataDog/datadog-agent/pkg/security/agent"
	"github.com/DataDog/datadog-agent/pkg/security/common"
	secconfig "github.com/DataDog/datadog-agent/pkg/security/config"
	pconfig "github.com/DataDog/datadog-agent/pkg/security/probe/config"
	"github.com/DataDog/datadog-agent/pkg/security/probe/kfilters"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	"github.com/DataDog/datadog-agent/pkg/security/reporter"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
	"github.com/DataDog/datadog-agent/pkg/version"
	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"
)

// Commands returns the config commands
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	runtimeCmd := &cobra.Command{
		Use:   "runtime",
		Short: "runtime Agent utility commands",
	}

	runtimeCmd.AddCommand(commonPolicyCommands(globalParams)...)
	runtimeCmd.AddCommand(selfTestCommands(globalParams)...)
	runtimeCmd.AddCommand(activityDumpCommands(globalParams)...)
	runtimeCmd.AddCommand(securityProfileCommands(globalParams)...)
	runtimeCmd.AddCommand(processCacheCommands(globalParams)...)
	runtimeCmd.AddCommand(networkNamespaceCommands(globalParams)...)
	runtimeCmd.AddCommand(discardersCommands(globalParams)...)

	// Deprecated
	runtimeCmd.AddCommand(checkPoliciesCommands(globalParams)...)
	runtimeCmd.AddCommand(reloadPoliciesCommands(globalParams)...)

	return []*cobra.Command{runtimeCmd}
}

type checkPoliciesCliParams struct {
	*command.GlobalParams

	dir                      string
	evaluateAllPolicySources bool
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

	dir       string
	ruleID    string
	eventFile string
	debug     bool
}

func evalCommands(globalParams *command.GlobalParams) []*cobra.Command {
	evalArgs := &evalCliParams{
		GlobalParams: globalParams,
	}

	evalCmd := &cobra.Command{
		Use:   "eval",
		Short: "Evaluate given event data against the give rule",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(evalRule,
				fx.Supply(evalArgs),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewSecurityAgentParams(globalParams.ConfigFilePaths),
					LogParams:    log.LogForOneShot(command.LoggerName, "off", false)}),
				core.Bundle,
			)
		},
	}

	evalCmd.Flags().StringVar(&evalArgs.dir, flags.PoliciesDir, pkgconfig.DefaultRuntimePoliciesDir, "Path to policies directory")
	evalCmd.Flags().StringVar(&evalArgs.ruleID, flags.RuleID, "", "Rule ID to evaluate")
	_ = evalCmd.MarkFlagRequired(flags.RuleID)
	evalCmd.Flags().StringVar(&evalArgs.eventFile, flags.EventFile, "", "File of the event data")
	_ = evalCmd.MarkFlagRequired(flags.EventFile)
	evalCmd.Flags().BoolVar(&evalArgs.debug, flags.Debug, false, "Display an event dump if the evaluation fail")

	return []*cobra.Command{evalCmd}
}

func commonCheckPoliciesCommands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &checkPoliciesCliParams{
		GlobalParams: globalParams,
	}

	commonCheckPoliciesCmd := &cobra.Command{
		Use:   "check",
		Short: "Check policies and return a report",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(checkPolicies,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewSecurityAgentParams(globalParams.ConfigFilePaths),
					LogParams:    log.LogForOneShot(command.LoggerName, "off", false)}),
				core.Bundle,
			)
		},
	}

	commonCheckPoliciesCmd.Flags().StringVar(&cliParams.dir, flags.PoliciesDir, pkgconfig.DefaultRuntimePoliciesDir, "Path to policies directory")
	commonCheckPoliciesCmd.Flags().BoolVar(&cliParams.evaluateAllPolicySources, flags.EvaluateLoadedPolicies, false, "Evaluate loaded policies")

	return []*cobra.Command{commonCheckPoliciesCmd}
}

func commonReloadPoliciesCommands(globalParams *command.GlobalParams) []*cobra.Command {
	commonReloadPoliciesCmd := &cobra.Command{
		Use:   "reload",
		Short: "Reload policies",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(reloadRuntimePolicies,
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewSecurityAgentParams(globalParams.ConfigFilePaths),
					LogParams:    log.LogForOneShot(command.LoggerName, "info", true)}),
				core.Bundle,
			)
		},
	}
	return []*cobra.Command{commonReloadPoliciesCmd}
}

func selfTestCommands(globalParams *command.GlobalParams) []*cobra.Command {
	selfTestCmd := &cobra.Command{
		Use:   "self-test",
		Short: "Run runtime self test",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(runRuntimeSelfTest,
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewSecurityAgentParams(globalParams.ConfigFilePaths),
					LogParams:    log.LogForOneShot(command.LoggerName, "info", true)}),
				core.Bundle,
			)
		},
	}

	return []*cobra.Command{selfTestCmd}
}

type downloadPolicyCliParams struct {
	*command.GlobalParams

	check      bool
	outputPath string
}

func downloadPolicyCommands(globalParams *command.GlobalParams) []*cobra.Command {
	downloadPolicyArgs := &downloadPolicyCliParams{
		GlobalParams: globalParams,
	}

	downloadPolicyCmd := &cobra.Command{
		Use:   "download",
		Short: "Download policies",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(downloadPolicy,
				fx.Supply(downloadPolicyArgs),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewSecurityAgentParams(globalParams.ConfigFilePaths),
					LogParams:    log.LogForOneShot(command.LoggerName, "off", false)}),
				core.Bundle,
			)
		},
	}

	downloadPolicyCmd.Flags().BoolVar(&downloadPolicyArgs.check, flags.Check, false, "Check policies after downloading")
	downloadPolicyCmd.Flags().StringVar(&downloadPolicyArgs.outputPath, flags.OutputPath, "", "Output path for downloaded policies")

	return []*cobra.Command{downloadPolicyCmd}
}

type processCacheDumpCliParams struct {
	*command.GlobalParams

	withArgs bool
}

func processCacheCommands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &processCacheDumpCliParams{
		GlobalParams: globalParams,
	}

	processCacheDumpCmd := &cobra.Command{
		Use:   "dump",
		Short: "dump the process cache",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(dumpProcessCache,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewSecurityAgentParams(globalParams.ConfigFilePaths),
					LogParams:    log.LogForOneShot(command.LoggerName, "info", true)}),
				core.Bundle,
			)
		},
	}
	processCacheDumpCmd.Flags().BoolVar(&cliParams.withArgs, flags.WithArgs, false, "add process arguments to the dump")

	processCacheCmd := &cobra.Command{
		Use:   "process-cache",
		Short: "process cache",
	}
	processCacheCmd.AddCommand(processCacheDumpCmd)

	return []*cobra.Command{processCacheCmd}
}

type dumpNetworkNamespaceCliParams struct {
	*command.GlobalParams

	snapshotInterfaces bool
}

func networkNamespaceCommands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &dumpNetworkNamespaceCliParams{
		GlobalParams: globalParams,
	}

	dumpNetworkNamespaceCmd := &cobra.Command{
		Use:   "dump",
		Short: "dumps the network namespaces held in cache",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(dumpNetworkNamespace,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewSecurityAgentParams(globalParams.ConfigFilePaths),
					LogParams:    log.LogForOneShot(command.LoggerName, "info", true)}),
				core.Bundle,
			)
		},
	}
	dumpNetworkNamespaceCmd.Flags().BoolVar(&cliParams.snapshotInterfaces, flags.SnapshotInterfaces, true, "snapshot the interfaces of each network namespace during the dump")

	networkNamespaceCmd := &cobra.Command{
		Use:   "network-namespace",
		Short: "network namespace command",
	}
	networkNamespaceCmd.AddCommand(dumpNetworkNamespaceCmd)

	return []*cobra.Command{networkNamespaceCmd}
}

func discardersCommands(globalParams *command.GlobalParams) []*cobra.Command {

	dumpDiscardersCmd := &cobra.Command{
		Use:   "dump",
		Short: "dump discarders",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(dumpDiscarders,
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewSecurityAgentParams(globalParams.ConfigFilePaths),
					LogParams:    log.LogForOneShot(command.LoggerName, "info", true)}),
				core.Bundle,
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

func dumpProcessCache(log log.Component, config config.Component, processCacheDumpArgs *processCacheDumpCliParams) error {
	client, err := secagent.NewRuntimeSecurityClient()
	if err != nil {
		return fmt.Errorf("unable to create a runtime security client instance: %w", err)
	}
	defer client.Close()

	filename, err := client.DumpProcessCache(processCacheDumpArgs.withArgs)
	if err != nil {
		return fmt.Errorf("unable to get a process cache dump: %w", err)
	}

	fmt.Printf("Process dump file: %s\n", filename)

	return nil
}

func dumpNetworkNamespace(log log.Component, config config.Component, dumpNetworkNamespaceArgs *dumpNetworkNamespaceCliParams) error {
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
		return fmt.Errorf("couldn't dump network namespaces: %w", err)
	}

	fmt.Printf("Network namespace dump: %s\n", resp.GetDumpFilename())
	fmt.Printf("Network namespace dump graph: %s\n", resp.GetGraphFilename())
	return nil
}

func printStorageRequestMessage(prefix string, storage *api.StorageRequestMessage) {
	fmt.Printf("%so file: %s\n", prefix, storage.GetFile())
	fmt.Printf("%s  format: %s\n", prefix, storage.GetFormat())
	fmt.Printf("%s  storage type: %s\n", prefix, storage.GetType())
	fmt.Printf("%s  compression: %v\n", prefix, storage.GetCompression())
}

func printSecurityActivityDumpMessage(prefix string, msg *api.ActivityDumpMessage) {
	fmt.Printf("%s- name: %s\n", prefix, msg.GetMetadata().GetName())
	fmt.Printf("%s  start: %s\n", prefix, msg.GetMetadata().GetStart())
	fmt.Printf("%s  timeout: %s\n", prefix, msg.GetMetadata().GetTimeout())
	if len(msg.GetMetadata().GetComm()) > 0 {
		fmt.Printf("%s  comm: %s\n", prefix, msg.GetMetadata().GetComm())
	}
	if len(msg.GetMetadata().GetContainerID()) > 0 {
		fmt.Printf("%s  container ID: %s\n", prefix, msg.GetMetadata().GetContainerID())
	}
	if len(msg.GetTags()) > 0 {
		fmt.Printf("%s  tags: %s\n", prefix, strings.Join(msg.GetTags(), ", "))
	}
	fmt.Printf("%s  differentiate args: %v\n", prefix, msg.GetMetadata().GetDifferentiateArgs())
	printActivityTreeStats(prefix, msg.GetStats())
	if len(msg.GetStorage()) > 0 {
		fmt.Printf("%s  storage:\n", prefix)
		for _, storage := range msg.GetStorage() {
			printStorageRequestMessage(prefix+"\t", storage)
		}
	}
}

func newAgentVersionFilter() (*rules.AgentVersionFilter, error) {
	agentVersion, err := utils.GetAgentSemverVersion()
	if err != nil {
		return nil, err
	}

	return rules.NewAgentVersionFilter(agentVersion)
}

func checkPolicies(log log.Component, config config.Component, args *checkPoliciesCliParams) error {
	if args.evaluateAllPolicySources {
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

func newDefaultEvent() eval.Event {
	return model.NewDefaultEvent()
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

	ruleOpts, evalOpts := rules.NewEvalOpts(enabled)

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

	ruleSet := rules.NewRuleSet(&model.Model{}, newDefaultEvent, ruleOpts, evalOpts)
	evaluationSet, err := rules.NewEvaluationSet([]*rules.RuleSet{ruleSet})
	if err != nil {
		return err
	}
	if err := evaluationSet.LoadPolicies(loader, loaderOpts); err.ErrorOrNil() != nil {
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

func evalRule(log log.Component, config config.Component, evalArgs *evalCliParams) error {
	policiesDir := evalArgs.dir

	// enabled all the rules
	enabled := map[eval.EventType]bool{"*": true}

	ruleOpts, evalOpts := rules.NewEvalOpts(enabled)
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

	ruleSet := rules.NewRuleSet(&model.Model{}, newDefaultEvent, ruleOpts, evalOpts)
	evaluationSet, err := rules.NewEvaluationSet([]*rules.RuleSet{ruleSet})
	if err != nil {
		return err
	}

	if err := evaluationSet.LoadPolicies(loader, loaderOpts); err.ErrorOrNil() != nil {
		return err
	}

	event, err := eventDataFromJSON(evalArgs.eventFile)
	if err != nil {
		return err
	}

	report := EvalReport{
		Event: event,
	}

	approvers, err := ruleSet.GetApprovers(kfilters.GetCapababilities())
	if err != nil {
		report.Error = err
	} else {
		report.Approvers = approvers
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

func runRuntimeSelfTest(log log.Component, config config.Component) error {
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

func reloadRuntimePolicies(log log.Component, config config.Component) error {
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

// StartRuntimeSecurity starts runtime security
func StartRuntimeSecurity(log log.Component, config config.Component, hostname string, stopper startstop.Stopper, statsdClient *ddgostatsd.Client, senderManager sender.SenderManager) (*secagent.RuntimeSecurityAgent, error) {
	enabled := config.GetBool("runtime_security_config.enabled")
	if !enabled {
		log.Info("Datadog runtime security agent disabled by config")
		return nil, nil
	}

	// start/stop order is important, agent need to be stopped first and started after all the others
	// components
	agent, err := secagent.NewRuntimeSecurityAgent(senderManager, hostname, secagent.RSAOptions{
		LogProfiledWorkloads:    config.GetBool("runtime_security_config.log_profiled_workloads"),
		IgnoreDDAgentContainers: config.GetBool("runtime_security_config.telemetry.ignore_dd_agent_containers"),
	})
	if err != nil {
		return nil, fmt.Errorf("unable to create a runtime security agent instance: %w", err)
	}
	stopper.Add(agent)

	useSecRuntimeTrack := config.GetBool("runtime_security_config.use_secruntime_track")
	endpoints, ctx, err := common.NewLogContextRuntime(useSecRuntimeTrack)
	if err != nil {
		_ = log.Error(err)
	}
	stopper.Add(ctx)

	runPath := config.GetString("runtime_security_config.run_path")
	reporter, err := reporter.NewCWSReporter(runPath, stopper, endpoints, ctx)
	if err != nil {
		return nil, err
	}

	agent.Start(reporter, endpoints)

	log.Info("Datadog runtime security agent is now running")

	return agent, nil
}

func downloadPolicy(log log.Component, config config.Component, downloadPolicyArgs *downloadPolicyCliParams) error {
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

	downloadURL := fmt.Sprintf("https://api.%s/api/v2/security/cloud_workload/policy/download", site)
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
	res, err := httputils.Get(ctx, downloadURL, headers, 10*time.Second)
	if err != nil {
		return err
	}
	resBytes := []byte(res)

	tempDir, err := os.MkdirTemp("", "policy_check")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	tempOutputPath := path.Join(tempDir, "check.policy")
	if err := os.WriteFile(tempOutputPath, resBytes, 0644); err != nil {
		return err
	}

	if downloadPolicyArgs.check {
		if err := checkPolicies(log, config, &checkPoliciesCliParams{dir: tempDir}); err != nil {
			return err
		}
	}

	_, err = outputWriter.Write(resBytes)
	if err != nil {
		return err
	}

	if outputFile != nil {
		return outputFile.Close()
	}

	return err
}

func dumpDiscarders(log log.Component, config config.Component) error {
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
