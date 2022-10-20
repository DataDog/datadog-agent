// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package app

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
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/probe/dump"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	cwsIntakeOrigin config.IntakeOrigin = "cloud-workload-security"
)

var (
	runtimeCmd = &cobra.Command{
		Use:   "runtime",
		Short: "runtime Agent utility commands",
	}

	checkPoliciesCmd = &cobra.Command{
		Use:        "check-policies",
		Short:      "check policies and return a report",
		RunE:       checkPolicies,
		Deprecated: "please use `security-agent runtime policy check` instead",
	}

	checkPoliciesArgs = struct {
		dir string
	}{}

	evalArgs = struct {
		dir       string
		ruleID    string
		eventFile string
		debug     bool
	}{}

	networkNamespaceCmd = &cobra.Command{
		Use:   "network-namespace",
		Short: "network namespace command",
	}

	dumpNetworkNamespaceCmd = &cobra.Command{
		Use:   "dump",
		Short: "dumps the network namespaces held in cache",
		RunE:  dumpNetworkNamespace,
	}

	dumpNetworkNamespaceArgs = struct {
		snapshotInterfaces bool
	}{}

	processCacheCmd = &cobra.Command{
		Use:   "process-cache",
		Short: "process cache",
	}

	processCacheDumpCmd = &cobra.Command{
		Use:   "dump",
		Short: "dump the process cache",
		RunE:  dumpProcessCache,
	}

	processCacheDumpArgs = struct {
		withArgs bool
	}{}

	activityDumpCmd = &cobra.Command{
		Use:   "activity-dump",
		Short: "activity dump command",
	}

	activityDumpArgs = struct {
		comm                     string
		file                     string
		timeout                  int
		differentiateArgs        bool
		localStorageDirectory    string
		localStorageFormats      []string
		localStorageCompression  bool
		remoteStorageFormats     []string
		remoteStorageCompression bool
		remoteRequest            bool
	}{}

	activityDumpGenerateCmd = &cobra.Command{
		Use:   "generate",
		Short: "generate command for activity dumps",
	}

	activityDumpGenerateDumpCmd = &cobra.Command{
		Use:   "dump",
		Short: "generate an activity dump",
		RunE:  generateActivityDump,
	}

	activityDumpGenerateEncodingCmd = &cobra.Command{
		Use:   "encoding",
		Short: "encode an activity dump to the requested formats",
		RunE:  generateEncodingFromActivityDump,
	}

	activityDumpStopCmd = &cobra.Command{
		Use:   "stop",
		Short: "stops the first activity dump that matches the provided selector",
		RunE:  stopActivityDump,
	}

	activityDumpListCmd = &cobra.Command{
		Use:   "list",
		Short: "get the list of running activity dumps",
		RunE:  listActivityDumps,
	}

	selfTestCmd = &cobra.Command{
		Use:   "self-test",
		Short: "Run runtime self test",
		RunE:  runRuntimeSelfTest,
	}

	reloadPoliciesCmd = &cobra.Command{
		Use:        "reload",
		Short:      "Reload policies",
		RunE:       reloadRuntimePolicies,
		Deprecated: "please use `security-agent runtime policy reload` instead",
	}

	commonReloadPoliciesCmd = &cobra.Command{
		Use:   "reload",
		Short: "Reload policies",
		RunE:  reloadRuntimePolicies,
	}

	commonCheckPoliciesCmd = &cobra.Command{
		Use:   "check",
		Short: "Check policies and return a report",
		RunE:  checkPolicies,
	}

	evalCmd = &cobra.Command{
		Use:   "eval",
		Short: "Evaluate given event data against the give rule",
		RunE:  evalRule,
	}

	downloadPolicyCmd = &cobra.Command{
		Use:   "download",
		Short: "Download policies",
		RunE:  downloadPolicy,
	}

	downloadPolicyArgs = struct {
		check      bool
		outputPath string
	}{}

	commonPolicyCmd = &cobra.Command{
		Use:   "policy",
		Short: "Policy related commands",
	}

	/*Discarders*/
	discardersCmd = &cobra.Command{
		Use:   "discarders",
		Short: "discarders commands",
	}

	dumpDiscardersCmd = &cobra.Command{
		Use:   "dump",
		Short: "dump discarders",
		RunE:  dumpDiscarders,
	}
)

func init() {
	processCacheDumpCmd.Flags().BoolVar(&processCacheDumpArgs.withArgs, "with-args", false, "add process arguments to the dump")

	activityDumpGenerateDumpCmd.Flags().StringVar(
		&activityDumpArgs.comm,
		"comm",
		"",
		"a process command can be used to filter the activity dump from a specific process.",
	)
	activityDumpGenerateDumpCmd.Flags().IntVar(
		&activityDumpArgs.timeout,
		"timeout",
		60,
		"timeout for the activity dump in minutes",
	)
	activityDumpGenerateDumpCmd.Flags().BoolVar(
		&activityDumpArgs.differentiateArgs,
		"differentiate-args",
		true,
		"add the arguments in the process node merge algorithm",
	)
	activityDumpGenerateDumpCmd.Flags().StringVar(
		&activityDumpArgs.localStorageDirectory,
		"output",
		"/tmp/activity_dumps/",
		"local storage output directory",
	)
	activityDumpGenerateDumpCmd.Flags().BoolVar(
		&activityDumpArgs.localStorageCompression,
		"compression",
		false,
		"defines if the local storage output should be compressed before persisting the data to disk",
	)
	activityDumpGenerateDumpCmd.Flags().StringArrayVar(
		&activityDumpArgs.localStorageFormats,
		"format",
		[]string{},
		fmt.Sprintf("local storage output formats. Available options are %v.", dump.AllStorageFormats()),
	)
	activityDumpGenerateDumpCmd.Flags().BoolVar(
		&activityDumpArgs.remoteStorageCompression,
		"remote-compression",
		true,
		"defines if the remote storage output should be compressed before sending the data",
	)
	activityDumpGenerateDumpCmd.Flags().StringArrayVar(
		&activityDumpArgs.remoteStorageFormats,
		"remote-format",
		[]string{},
		fmt.Sprintf("remote storage output formats. Available options are %v.", dump.AllStorageFormats()),
	)

	activityDumpStopCmd.Flags().StringVar(
		&activityDumpArgs.comm,
		"comm",
		"",
		"a process command can be used to filter the activity dump from a specific process.",
	)

	activityDumpGenerateEncodingCmd.Flags().StringVar(
		&activityDumpArgs.file,
		"input",
		"",
		"path to the activity dump file",
	)
	_ = activityDumpGenerateEncodingCmd.MarkFlagRequired("input")
	activityDumpGenerateEncodingCmd.Flags().StringVar(
		&activityDumpArgs.localStorageDirectory,
		"output",
		"/tmp/activity_dumps/",
		"local storage output directory",
	)
	activityDumpGenerateEncodingCmd.Flags().BoolVar(
		&activityDumpArgs.localStorageCompression,
		"compression",
		false,
		"defines if the local storage output should be compressed before persisting the data to disk",
	)
	activityDumpGenerateEncodingCmd.Flags().StringArrayVar(
		&activityDumpArgs.localStorageFormats,
		"format",
		[]string{},
		fmt.Sprintf("local storage output formats. Available options are %v.", dump.AllStorageFormats()),
	)
	activityDumpGenerateEncodingCmd.Flags().BoolVar(
		&activityDumpArgs.remoteStorageCompression,
		"remote-compression",
		true,
		"defines if the remote storage output should be compressed before sending the data",
	)
	activityDumpGenerateEncodingCmd.Flags().StringArrayVar(
		&activityDumpArgs.remoteStorageFormats,
		"remote-format",
		[]string{},
		fmt.Sprintf("remote storage output formats. Available options are %v.", dump.AllStorageFormats()),
	)
	activityDumpGenerateEncodingCmd.Flags().BoolVar(
		&activityDumpArgs.remoteRequest,
		"remote",
		false,
		"when set, the transcoding will be done by system-probe instead of the current security-agent instance",
	)

	processCacheCmd.AddCommand(processCacheDumpCmd)
	runtimeCmd.AddCommand(processCacheCmd)

	activityDumpGenerateCmd.AddCommand(activityDumpGenerateDumpCmd)
	activityDumpGenerateCmd.AddCommand(activityDumpGenerateEncodingCmd)

	activityDumpCmd.AddCommand(activityDumpGenerateCmd)
	activityDumpCmd.AddCommand(activityDumpListCmd)
	activityDumpCmd.AddCommand(activityDumpStopCmd)
	runtimeCmd.AddCommand(activityDumpCmd)

	runtimeCmd.AddCommand(checkPoliciesCmd)
	checkPoliciesCmd.Flags().StringVar(&checkPoliciesArgs.dir, "policies-dir", coreconfig.DefaultRuntimePoliciesDir, "Path to policies directory")

	commonPolicyCmd.AddCommand(evalCmd)
	evalCmd.Flags().StringVar(&evalArgs.dir, "policies-dir", coreconfig.DefaultRuntimePoliciesDir, "Path to policies directory")
	evalCmd.Flags().StringVar(&evalArgs.ruleID, "rule-id", "", "Rule ID to evaluate")
	_ = evalCmd.MarkFlagRequired("rule-id")
	evalCmd.Flags().StringVar(&evalArgs.eventFile, "event-file", "", "File of the event data")
	_ = evalCmd.MarkFlagRequired("event-file")
	evalCmd.Flags().BoolVar(&evalArgs.debug, "debug", false, "Display an event dump if the evaluation fail")

	runtimeCmd.AddCommand(selfTestCmd)
	runtimeCmd.AddCommand(reloadPoliciesCmd)

	downloadPolicyCmd.Flags().BoolVar(&downloadPolicyArgs.check, "check", false, "Check policies after downloading")
	downloadPolicyCmd.Flags().StringVar(&downloadPolicyArgs.outputPath, "output-path", "", "Output path for downloaded policies")
	commonPolicyCmd.AddCommand(downloadPolicyCmd)

	commonCheckPoliciesCmd.Flags().StringVar(&checkPoliciesArgs.dir, "policies-dir", coreconfig.DefaultRuntimePoliciesDir, "Path to policies directory")
	commonPolicyCmd.AddCommand(commonCheckPoliciesCmd)

	commonPolicyCmd.AddCommand(commonReloadPoliciesCmd)
	runtimeCmd.AddCommand(commonPolicyCmd)

	dumpNetworkNamespaceCmd.Flags().BoolVar(&dumpNetworkNamespaceArgs.snapshotInterfaces, "snapshot-interfaces", true, "snapshot the interfaces of each network namespace during the dump")
	networkNamespaceCmd.AddCommand(dumpNetworkNamespaceCmd)
	runtimeCmd.AddCommand(networkNamespaceCmd)

	/*Discarders*/
	discardersCmd.AddCommand(dumpDiscardersCmd)
	runtimeCmd.AddCommand(discardersCmd)
}

func dumpProcessCache(cmd *cobra.Command, args []string) error {
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

func dumpNetworkNamespace(cmd *cobra.Command, args []string) error {
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

func parseStorageRequest() (*api.StorageRequestParams, error) {
	// parse local storage formats
	_, err := dump.ParseStorageFormats(activityDumpArgs.localStorageFormats)
	if err != nil {
		return nil, fmt.Errorf("couldn't parse local storage formats %v: %v", activityDumpArgs.localStorageFormats, err)
	}

	// parse remote storage formats
	_, err = dump.ParseStorageFormats(activityDumpArgs.remoteStorageFormats)
	if err != nil {
		return nil, fmt.Errorf("couldn't parse remote storage formats %v: %v", activityDumpArgs.remoteStorageFormats, err)
	}
	return &api.StorageRequestParams{
		LocalStorageDirectory:    activityDumpArgs.localStorageDirectory,
		LocalStorageCompression:  activityDumpArgs.localStorageCompression,
		LocalStorageFormats:      activityDumpArgs.localStorageFormats,
		RemoteStorageCompression: activityDumpArgs.remoteStorageCompression,
		RemoteStorageFormats:     activityDumpArgs.remoteStorageFormats,
	}, nil
}

func generateActivityDump(cmd *cobra.Command, args []string) error {
	client, err := secagent.NewRuntimeSecurityClient()
	if err != nil {
		return fmt.Errorf("unable to create a runtime security client instance: %w", err)
	}
	defer client.Close()

	storage, err := parseStorageRequest()
	if err != nil {
		return err
	}

	output, err := client.GenerateActivityDump(&api.ActivityDumpParams{
		Comm:              activityDumpArgs.comm,
		Timeout:           int32(activityDumpArgs.timeout),
		DifferentiateArgs: activityDumpArgs.differentiateArgs,
		Storage:           storage,
	})
	if err != nil {
		return fmt.Errorf("unable send request to system-probe: %w", err)
	}
	if len(output.Error) > 0 {
		return fmt.Errorf("activity dump generation request failed: %s", output.Error)
	}

	printSecurityActivityDumpMessage("", output)
	return nil
}

func listActivityDumps(cmd *cobra.Command, args []string) error {
	client, err := secagent.NewRuntimeSecurityClient()
	if err != nil {
		return fmt.Errorf("unable to create a runtime security client instance: %w", err)
	}
	defer client.Close()

	output, err := client.ListActivityDumps()
	if err != nil {
		return fmt.Errorf("unable send request to system-probe: %w", err)
	}
	if len(output.Error) > 0 {
		return fmt.Errorf("activity dump list request failed: %s", output.Error)
	}

	if len(output.Dumps) > 0 {
		fmt.Println("active dumps:")
		for _, d := range output.Dumps {
			printSecurityActivityDumpMessage("\t", d)
		}
	} else {
		fmt.Println("no active dumps found")
	}

	return nil
}

func stopActivityDump(cmd *cobra.Command, args []string) error {
	client, err := secagent.NewRuntimeSecurityClient()
	if err != nil {
		return fmt.Errorf("unable to create a runtime security client instance: %w", err)
	}
	defer client.Close()

	output, err := client.StopActivityDump(activityDumpArgs.comm)
	if err != nil {
		return fmt.Errorf("unable send request to system-probe: %w", err)
	}
	if len(output.Error) > 0 {
		return fmt.Errorf("activity dump stop request failed: %s", output.Error)
	}

	fmt.Println("done!")
	return nil
}

func generateEncodingFromActivityDump(cmd *cobra.Command, args []string) error {
	var output *api.TranscodingRequestMessage

	if activityDumpArgs.remoteRequest {
		// send the encoding request to system-probe
		client, err := secagent.NewRuntimeSecurityClient()
		if err != nil {
			return fmt.Errorf("encoding generation failed: %w", err)
		}
		defer client.Close()

		// parse encoding request
		storage, err := parseStorageRequest()
		if err != nil {
			return err
		}

		output, err = client.GenerateEncoding(&api.TranscodingRequestParams{
			ActivityDumpFile: activityDumpArgs.file,
			Storage:          storage,
		})
		if err != nil {
			return fmt.Errorf("couldn't send request to system-probe: %w", err)
		}

	} else {
		// encoding request will be handled locally
		ad := sprobe.NewEmptyActivityDump()

		// open and parse input file
		if err := ad.Decode(activityDumpArgs.file); err != nil {
			return err
		}
		parsedRequests, err := parseStorageRequest()
		if err != nil {
			return err
		}

		storageRequests, err := dump.ParseStorageRequests(parsedRequests)
		if err != nil {
			return fmt.Errorf("couldn't parse transcoding request for [%s]: %v", ad.GetSelectorStr(), err)
		}
		for _, request := range storageRequests {
			ad.AddStorageRequest(request)
		}

		storage, err := sprobe.NewActivityDumpStorageManager(nil)
		if err != nil {
			return fmt.Errorf("couldn't instantiate storage manager: %w", err)
		}

		err = storage.Persist(ad)
		if err != nil {
			return fmt.Errorf("couldn't persist dump from %s: %w", activityDumpArgs.file, err)
		}

		output = ad.ToTranscodingRequestMessage()
	}

	if len(output.GetError()) > 0 {
		return fmt.Errorf("encoding generation failed: %s", output.GetError())
	}
	if len(output.GetStorage()) > 0 {
		fmt.Printf("encoding generation succeeded:\n")
		for _, storage := range output.GetStorage() {
			printStorageRequestMessage("\t", storage)
		}
	} else {
		fmt.Println("encoding generation succeeded: empty output")
	}
	return nil
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
		WithReservedRuleIDs(sprobe.AllCustomRuleIDs()).
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

func checkPolicies(cmd *cobra.Command, args []string) error {
	return checkPoliciesInner(checkPoliciesArgs.dir)
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

func evalRule(cmd *cobra.Command, args []string) error {
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
		WithReservedRuleIDs(sprobe.AllCustomRuleIDs()).
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

func runRuntimeSelfTest(cmd *cobra.Command, args []string) error {
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

func reloadRuntimePolicies(cmd *cobra.Command, args []string) error {
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
	return newLogContext(logsConfigComplianceKeys, "runtime-security-http-intake.logs.", "logs", cwsIntakeOrigin, config.DefaultIntakeProtocol)
}

func startRuntimeSecurity(hostname string, stopper startstop.Stopper, statsdClient *ddgostatsd.Client) (*secagent.RuntimeSecurityAgent, error) {
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

func downloadPolicy(cmd *cobra.Command, args []string) error {
	apiKey := coreconfig.Datadog.GetString("api_key")
	appKey := coreconfig.Datadog.GetString("app_key")

	if apiKey == "" {
		return errors.New("API key is empty")
	}

	if appKey == "" {
		return errors.New("application key is empty")
	}

	site := coreconfig.Datadog.GetString("site")
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

func dumpDiscarders(cmd *cobra.Command, args []string) error {
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
