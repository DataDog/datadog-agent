// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

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
	"unsafe"

	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"
	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/security-agent/app/common"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/pkg/compliance/event"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	secagent "github.com/DataDog/datadog-agent/pkg/security/agent"
	"github.com/DataDog/datadog-agent/pkg/security/api"
	secconfig "github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/events"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
	"github.com/DataDog/datadog-agent/pkg/version"

	compconfig "github.com/DataDog/datadog-agent/comp/core/config"
	complog "github.com/DataDog/datadog-agent/comp/core/log"
)

const (
	cwsIntakeOrigin config.IntakeOrigin = "cloud-workload-security"
)

func Commands(globalParams *common.GlobalParams) []*cobra.Command {
	runtimeCmd := &cobra.Command{
		Use:   "runtime",
		Short: "runtime Agent utility commands",
	}

	runtimeCmd.AddCommand(checkPoliciesCommands(globalParams)...)
	runtimeCmd.AddCommand(reloadPoliciesCommands(globalParams)...)
	runtimeCmd.AddCommand(commonPolicyCommands(globalParams)...)
	runtimeCmd.AddCommand(selfTestCommands(globalParams)...)
	runtimeCmd.AddCommand(activityDumpCommands(globalParams)...)
	runtimeCmd.AddCommand(processCacheCommands(globalParams)...)
	runtimeCmd.AddCommand(networkNamespaceCommands(globalParams)...)
	runtimeCmd.AddCommand(discardersCommands(globalParams)...)

	return []*cobra.Command{runtimeCmd}
}

type checkPoliciesCliParams struct {
	*common.GlobalParams

	dir string
}

func checkPoliciesCommands(globalParams *common.GlobalParams) []*cobra.Command {
	cliParams := &checkPoliciesCliParams{
		GlobalParams: globalParams,
	}

	checkPoliciesCmd := &cobra.Command{
		Use:   "check-policies",
		Short: "check policies and return a report",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(checkPolicies,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: compconfig.NewSecurityAgentParams(globalParams.ConfPathArray),
					LogParams:    complog.LogForOneShot(common.LoggerName, "off", false)}),
				core.Bundle,
			)
		},
		Deprecated: "please use `security-agent runtime policy check` instead",
	}

	checkPoliciesCmd.Flags().StringVar(&cliParams.dir, "policies-dir", coreconfig.DefaultRuntimePoliciesDir, "Path to policies directory")

	return []*cobra.Command{checkPoliciesCmd}
}

func reloadPoliciesCommands(globalParams *common.GlobalParams) []*cobra.Command {
	reloadPoliciesCmd := &cobra.Command{
		Use:   "reload",
		Short: "Reload policies",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(reloadRuntimePolicies,
				fx.Supply(core.BundleParams{
					ConfigParams: compconfig.NewSecurityAgentParams(globalParams.ConfPathArray),
					LogParams:    complog.LogForOneShot(common.LoggerName, "info", true)}),
				core.Bundle,
			)
		},
		Deprecated: "please use `security-agent runtime policy reload` instead",
	}
	return []*cobra.Command{reloadPoliciesCmd}
}

func commonPolicyCommands(globalParams *common.GlobalParams) []*cobra.Command {
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
	*common.GlobalParams

	dir       string
	ruleID    string
	eventFile string
	debug     bool
}

func evalCommands(globalParams *common.GlobalParams) []*cobra.Command {
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
					ConfigParams: compconfig.NewSecurityAgentParams(globalParams.ConfPathArray),
					LogParams:    complog.LogForOneShot(common.LoggerName, "off", false)}),
				core.Bundle,
			)
		},
	}

	evalCmd.Flags().StringVar(&evalArgs.dir, "policies-dir", coreconfig.DefaultRuntimePoliciesDir, "Path to policies directory")
	evalCmd.Flags().StringVar(&evalArgs.ruleID, "rule-id", "", "Rule ID to evaluate")
	_ = evalCmd.MarkFlagRequired("rule-id")
	evalCmd.Flags().StringVar(&evalArgs.eventFile, "event-file", "", "File of the event data")
	_ = evalCmd.MarkFlagRequired("event-file")
	evalCmd.Flags().BoolVar(&evalArgs.debug, "debug", false, "Display an event dump if the evaluation fail")

	return []*cobra.Command{evalCmd}
}

func commonCheckPoliciesCommands(globalParams *common.GlobalParams) []*cobra.Command {
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
					ConfigParams: compconfig.NewSecurityAgentParams(globalParams.ConfPathArray),
					LogParams:    complog.LogForOneShot(common.LoggerName, "off", false)}),
				core.Bundle,
			)
		},
	}

	commonCheckPoliciesCmd.Flags().StringVar(&cliParams.dir, "policies-dir", coreconfig.DefaultRuntimePoliciesDir, "Path to policies directory")

	return []*cobra.Command{commonCheckPoliciesCmd}
}

func commonReloadPoliciesCommands(globalParams *common.GlobalParams) []*cobra.Command {
	commonReloadPoliciesCmd := &cobra.Command{
		Use:   "reload",
		Short: "Reload policies",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(reloadRuntimePolicies,
				fx.Supply(core.BundleParams{
					ConfigParams: compconfig.NewSecurityAgentParams(globalParams.ConfPathArray),
					LogParams:    complog.LogForOneShot(common.LoggerName, "info", true)}),
				core.Bundle,
			)
		},
	}
	return []*cobra.Command{commonReloadPoliciesCmd}
}

func selfTestCommands(globalParams *common.GlobalParams) []*cobra.Command {
	selfTestCmd := &cobra.Command{
		Use:   "self-test",
		Short: "Run runtime self test",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(runRuntimeSelfTest,
				fx.Supply(core.BundleParams{
					ConfigParams: compconfig.NewSecurityAgentParams(globalParams.ConfPathArray),
					LogParams:    complog.LogForOneShot(common.LoggerName, "info", true)}),
				core.Bundle,
			)
		},
	}

	return []*cobra.Command{selfTestCmd}
}

type downloadPolicyCliParams struct {
	*common.GlobalParams

	check      bool
	outputPath string
}

func downloadPolicyCommands(globalParams *common.GlobalParams) []*cobra.Command {
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
					ConfigParams: compconfig.NewSecurityAgentParams(globalParams.ConfPathArray),
					LogParams:    complog.LogForOneShot(common.LoggerName, "off", false)}),
				core.Bundle,
			)
		},
	}

	downloadPolicyCmd.Flags().BoolVar(&downloadPolicyArgs.check, "check", false, "Check policies after downloading")
	downloadPolicyCmd.Flags().StringVar(&downloadPolicyArgs.outputPath, "output-path", "", "Output path for downloaded policies")

	return []*cobra.Command{downloadPolicyCmd}
}

type processCacheDumpCliParams struct {
	*common.GlobalParams

	withArgs bool
}

func processCacheCommands(globalParams *common.GlobalParams) []*cobra.Command {
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
					ConfigParams: compconfig.NewSecurityAgentParams(globalParams.ConfPathArray),
					LogParams:    complog.LogForOneShot(common.LoggerName, "info", true)}),
				core.Bundle,
			)
		},
	}
	processCacheDumpCmd.Flags().BoolVar(&cliParams.withArgs, "with-args", false, "add process arguments to the dump")

	processCacheCmd := &cobra.Command{
		Use:   "process-cache",
		Short: "process cache",
	}
	processCacheCmd.AddCommand(processCacheDumpCmd)

	return []*cobra.Command{processCacheCmd}
}

type dumpNetworkNamespaceCliParams struct {
	*common.GlobalParams

	snapshotInterfaces bool
}

func networkNamespaceCommands(globalParams *common.GlobalParams) []*cobra.Command {
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
					ConfigParams: compconfig.NewSecurityAgentParams(globalParams.ConfPathArray),
					LogParams:    complog.LogForOneShot(common.LoggerName, "info", true)}),
				core.Bundle,
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

func discardersCommands(globalParams *common.GlobalParams) []*cobra.Command {

	dumpDiscardersCmd := &cobra.Command{
		Use:   "dump",
		Short: "dump discarders",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(dumpDiscarders,
				fx.Supply(core.BundleParams{
					ConfigParams: compconfig.NewSecurityAgentParams(globalParams.ConfPathArray),
					LogParams:    complog.LogForOneShot(common.LoggerName, "info", true)}),
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

func dumpProcessCache(log complog.Component, config compconfig.Component, processCacheDumpArgs *processCacheDumpCliParams) error {
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

func dumpNetworkNamespace(log complog.Component, config compconfig.Component, dumpNetworkNamespaceArgs *dumpNetworkNamespaceCliParams) error {
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

func checkPoliciesInner(dir string) error {
	cfg := &secconfig.Config{
		PoliciesDir:         dir,
		EnableKernelFilters: true,
		EnableApprovers:     true,
		EnableDiscarders:    true,
		PIDCacheSize:        1,
	}

	// enabled all the rules
	enabled := map[eval.EventType]bool{"*": true}

	var evalOpts eval.Opts
	evalOpts.
		WithConstants(model.SECLConstants).
		WithVariables(model.SECLVariables).
		WithLegacyFields(model.SECLLegacyFields)

	var opts rules.Opts
	opts.
		WithSupportedDiscarders(sprobe.SupportedDiscarders).
		WithEventTypeEnabled(enabled).
		WithReservedRuleIDs(events.AllCustomRuleIDs()).
		WithStateScopes(map[rules.Scope]rules.VariableProviderFactory{
			"process": func() rules.VariableProvider {
				return eval.NewScopedVariables(func(ctx *eval.Context) unsafe.Pointer {
					return unsafe.Pointer(&(*model.Event)(ctx.Object).ProcessContext)
				}, nil)
			},
		}).
		WithLogger(seclog.DefaultLogger)

	model := &model.Model{}
	ruleSet := rules.NewRuleSet(model, model.NewEvent, &opts, &evalOpts, &eval.MacroStore{})

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

	provider, err := rules.NewPoliciesDirProvider(cfg.PoliciesDir, false)
	if err != nil {
		return err
	}

	loader := rules.NewPolicyLoader(provider)

	if err := ruleSet.LoadPolicies(loader, loaderOpts); err.ErrorOrNil() != nil {
		return err
	}

	approvers, err := ruleSet.GetApprovers(sprobe.GetCapababilities())
	if err != nil {
		return err
	}

	rsa := sprobe.NewRuleSetApplier(cfg, nil)

	report, err := rsa.Apply(ruleSet, approvers)
	if err != nil {
		return err
	}

	content, _ := json.MarshalIndent(report, "", "\t")
	fmt.Printf("%s\n", string(content))

	return nil
}

func checkPolicies(log complog.Component, config compconfig.Component, args *checkPoliciesCliParams) error {
	return checkPoliciesInner(args.dir)
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

	kind := model.ParseEvalEventType(eventData.Type)
	if kind == model.UnknownEventType {
		return nil, errors.New("unknown event type")
	}

	m := &model.Model{}
	event := m.NewEventWithType(kind)
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

func evalRule(log complog.Component, config compconfig.Component, evalArgs *evalCliParams) error {
	cfg := &secconfig.Config{
		PoliciesDir:         evalArgs.dir,
		EnableKernelFilters: true,
		EnableApprovers:     true,
		EnableDiscarders:    true,
		PIDCacheSize:        1,
	}

	// enabled all the rules
	enabled := map[eval.EventType]bool{"*": true}

	var evalOpts eval.Opts
	evalOpts.
		WithConstants(model.SECLConstants).
		WithVariables(model.SECLVariables).
		WithLegacyFields(model.SECLLegacyFields)

	var opts rules.Opts
	opts.
		WithSupportedDiscarders(sprobe.SupportedDiscarders).
		WithEventTypeEnabled(enabled).
		WithReservedRuleIDs(events.AllCustomRuleIDs()).
		WithLogger(seclog.DefaultLogger)

	model := &model.Model{}
	ruleSet := rules.NewRuleSet(model, model.NewEvent, &opts, &evalOpts, &eval.MacroStore{})

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

	provider, err := rules.NewPoliciesDirProvider(cfg.PoliciesDir, false)
	if err != nil {
		return err
	}

	loader := rules.NewPolicyLoader(provider)

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

	approvers, err := ruleSet.GetApprovers(sprobe.GetCapababilities())
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

func runRuntimeSelfTest(log complog.Component, config compconfig.Component) error {
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

func reloadRuntimePolicies(log complog.Component, config compconfig.Component) error {
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

func newRuntimeReporter(stopper startstop.Stopper, sourceName, sourceType string, endpoints *config.Endpoints, context *client.DestinationsContext) (event.Reporter, error) {
	health := health.RegisterLiveness("runtime-security")

	// setup the auditor
	auditor := auditor.New(coreconfig.Datadog.GetString("runtime_security_config.run_path"), "runtime-security-registry.json", coreconfig.DefaultAuditorTTL, health)
	auditor.Start()
	stopper.Add(auditor)

	// setup the pipeline provider that provides pairs of processor and sender
	pipelineProvider := pipeline.NewProvider(config.NumberOfPipelines, auditor, &diagnostic.NoopMessageReceiver{}, nil, endpoints, context)
	pipelineProvider.Start()
	stopper.Add(pipelineProvider)

	logSource := sources.NewLogSource(
		sourceName,
		&config.LogsConfig{
			Type:   sourceType,
			Source: sourceName,
		},
	)
	return event.NewReporter(logSource, pipelineProvider.NextPipelineChan()), nil
}

// This function will only be used on Linux. The only platforms where the runtime agent runs
func newLogContextRuntime() (*config.Endpoints, *client.DestinationsContext, error) { // nolint: deadcode, unused
	logsConfigComplianceKeys := config.NewLogsConfigKeys("runtime_security_config.endpoints.", coreconfig.Datadog)
	return common.NewLogContext(logsConfigComplianceKeys, "runtime-security-http-intake.logs.", "logs", cwsIntakeOrigin, config.DefaultIntakeProtocol)
}

func StartRuntimeSecurity(hostname string, stopper startstop.Stopper, statsdClient *ddgostatsd.Client) (*secagent.RuntimeSecurityAgent, error) {
	enabled := coreconfig.Datadog.GetBool("runtime_security_config.enabled")
	if !enabled {
		log.Info("Datadog runtime security agent disabled by config")
		return nil, nil
	}

	// start/stop order is important, agent need to be stopped first and started after all the others
	// components
	agent, err := secagent.NewRuntimeSecurityAgent(hostname)
	if err != nil {
		return nil, fmt.Errorf("unable to create a runtime security agent instance: %w", err)
	}
	stopper.Add(agent)

	endpoints, context, err := newLogContextRuntime()
	if err != nil {
		log.Error(err)
	}
	stopper.Add(context)

	reporter, err := newRuntimeReporter(stopper, "runtime-security-agent", "runtime-security", endpoints, context)
	if err != nil {
		return nil, err
	}

	agent.Start(reporter, endpoints)

	log.Info("Datadog runtime security agent is now running")

	return agent, nil
}

func downloadPolicy(log complog.Component, config compconfig.Component, downloadPolicyArgs *downloadPolicyCliParams) error {
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
		if err := checkPoliciesInner(tempDir); err != nil {
			return err
		}
	}

	_, err = outputWriter.Write(resBytes)
	return err
}

func dumpDiscarders(log complog.Component, config compconfig.Component) error {
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
