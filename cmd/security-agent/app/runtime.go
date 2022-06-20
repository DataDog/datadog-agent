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
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/security-agent/common"
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
	seclog "github.com/DataDog/datadog-agent/pkg/security/log"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
	"github.com/DataDog/datadog-agent/pkg/version"
	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"
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
		comm              string
		file              string
		timeout           int
		withGraph         bool
		differentiateArgs bool
		outputDirectory   string
		outputFormat      string
		remote            bool
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

	activityDumpGenerateProfileCmd = &cobra.Command{
		Use:   "profile",
		Short: "generate a profile from an activity dump",
		RunE:  generateProfileFromActivityDump,
	}

	activityDumpGenerateGraphCmd = &cobra.Command{
		Use:   "graph",
		Short: "generate a graph from an activity dump",
		RunE:  generateGraphFromActivityDump,
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
		&activityDumpArgs.withGraph,
		"graph",
		false,
		"generate a graph from the generated dump",
	)
	activityDumpGenerateDumpCmd.Flags().BoolVar(
		&activityDumpArgs.differentiateArgs,
		"differentiate-args",
		false,
		"add the arguments in the process node merge algorithm",
	)
	activityDumpGenerateDumpCmd.Flags().StringVar(
		&activityDumpArgs.outputDirectory,
		"output",
		"/tmp/activity_dumps/",
		"output directory",
	)
	activityDumpGenerateDumpCmd.Flags().StringVar(
		&activityDumpArgs.outputFormat,
		"format",
		"msgp",
		"output format. Available options are \"msgp\" and \"json\".",
	)

	activityDumpStopCmd.Flags().StringVar(
		&activityDumpArgs.comm,
		"comm",
		"",
		"a process command can be used to filter the activity dump from a specific process.",
	)

	activityDumpGenerateProfileCmd.Flags().StringVar(
		&activityDumpArgs.file,
		"input",
		"",
		"path to the activity dump file",
	)
	_ = activityDumpGenerateProfileCmd.MarkFlagRequired("input")
	activityDumpGenerateProfileCmd.Flags().BoolVar(
		&activityDumpArgs.remote,
		"remote",
		false,
		"when set, the profile generation will be done by system-probe, otherwise the current security-agent process will generate the profile",
	)

	activityDumpGenerateGraphCmd.Flags().StringVar(
		&activityDumpArgs.file,
		"input",
		"",
		"path to the activity dump file",
	)
	_ = activityDumpGenerateProfileCmd.MarkFlagRequired("input")
	activityDumpGenerateGraphCmd.Flags().BoolVar(
		&activityDumpArgs.remote,
		"remote",
		false,
		"when set, the profile generation will be done by system-probe, otherwise the current security-agent process will generate the profile",
	)

	processCacheCmd.AddCommand(processCacheDumpCmd)
	runtimeCmd.AddCommand(processCacheCmd)

	activityDumpGenerateCmd.AddCommand(activityDumpGenerateDumpCmd)
	activityDumpGenerateCmd.AddCommand(activityDumpGenerateProfileCmd)
	activityDumpGenerateCmd.AddCommand(activityDumpGenerateGraphCmd)

	activityDumpCmd.AddCommand(activityDumpGenerateCmd)
	activityDumpCmd.AddCommand(activityDumpListCmd)
	activityDumpCmd.AddCommand(activityDumpStopCmd)
	runtimeCmd.AddCommand(activityDumpCmd)

	runtimeCmd.AddCommand(checkPoliciesCmd)
	checkPoliciesCmd.Flags().StringVar(&checkPoliciesArgs.dir, "policies-dir", coreconfig.DefaultRuntimePoliciesDir, "Path to policies directory")

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
}

func dumpProcessCache(cmd *cobra.Command, args []string) error {
	// Read configuration files received from the command line arguments '-c'
	if err := common.MergeConfigurationFiles("datadog", confPathArray, cmd.Flags().Lookup("cfgpath").Changed); err != nil {
		return err
	}

	client, err := secagent.NewRuntimeSecurityClient()
	if err != nil {
		return errors.Wrap(err, "unable to create a runtime security client instance")
	}
	defer client.Close()

	filename, err := client.DumpProcessCache(processCacheDumpArgs.withArgs)
	if err != nil {
		return errors.Wrap(err, "unable to get a process cache dump")
	}

	fmt.Printf("Process dump file: %s\n", filename)

	return nil
}

func printSecurityActivityDumpMessage(prefix string, msg *api.SecurityActivityDumpMessage) {
	fmt.Printf("%s- start: %s\n", prefix, msg.Start)
	fmt.Printf("%s  timeout: %s\n", prefix, msg.Timeout)
	fmt.Printf("%s  left: %s\n", prefix, msg.Left)
	if len(msg.OutputFilename) > 0 {
		fmt.Printf("%s  output filename: %s\n", prefix, msg.OutputFilename)
	}
	if len(msg.GraphFilename) > 0 {
		fmt.Printf("%s  graph filename: %s\n", prefix, msg.GraphFilename)
	}
	if len(msg.Comm) > 0 {
		fmt.Printf("%s  comm: %s\n", prefix, msg.Comm)
	}
	if len(msg.ContainerID) > 0 {
		fmt.Printf("%s  container ID: %s\n", prefix, msg.ContainerID)
	}
	if len(msg.Tags) > 0 {
		fmt.Printf("%s  tags: %s\n", prefix, strings.Join(msg.Tags, ", "))
	}
	fmt.Printf("%s  with graph: %v\n", prefix, msg.WithGraph)
	fmt.Printf("%s  differentiate args: %v\n", prefix, msg.DifferentiateArgs)
}

func generateActivityDump(cmd *cobra.Command, args []string) error {
	// Read configuration files received from the command line arguments '-c'
	if err := common.MergeConfigurationFiles("datadog", confPathArray, cmd.Flags().Lookup("cfgpath").Changed); err != nil {
		return err
	}

	if len(activityDumpArgs.outputDirectory) == 0 && activityDumpArgs.withGraph {
		return fmt.Errorf("the output directory cannot be empty if \"--graph\" is provided")
	}

	client, err := secagent.NewRuntimeSecurityClient()
	if err != nil {
		return errors.Wrap(err, "unable to create a runtime security client instance")
	}
	defer client.Close()

	output, err := client.GenerateActivityDump(activityDumpArgs.comm, int32(activityDumpArgs.timeout), activityDumpArgs.withGraph, activityDumpArgs.differentiateArgs, activityDumpArgs.outputDirectory, activityDumpArgs.outputFormat)
	if err != nil {
		return fmt.Errorf("unable send request to system-probe: %w", err)
	}
	if len(output.Error) > 0 {
		return fmt.Errorf("activity dump generation request failed: %s", output.Error)
	}

	printSecurityActivityDumpMessage("", output)
	return nil
}

func dumpNetworkNamespace(cmd *cobra.Command, args []string) error {
	// Read configuration files received from the command line arguments '-c'
	if err := common.MergeConfigurationFiles("datadog", confPathArray, cmd.Flags().Lookup("cfgpath").Changed); err != nil {
		return err
	}

	client, err := secagent.NewRuntimeSecurityClient()
	if err != nil {
		return errors.Wrap(err, "unable to create a runtime security client instance")
	}
	defer client.Close()

	resp, err := client.DumpNetworkNamespace(dumpNetworkNamespaceArgs.snapshotInterfaces)
	if err != nil {
		return errors.Wrap(err, "couldn't send network namespace cache dump request")
	}

	if len(resp.GetError()) > 0 {
		return fmt.Errorf("couldn't dump network namespaces: %w", err)
	}

	fmt.Printf("Network namespace dump: %s\n", resp.GetDumpFilename())
	fmt.Printf("Network namespace dump graph: %s\n", resp.GetGraphFilename())
	return nil
}

func listActivityDumps(cmd *cobra.Command, args []string) error {
	// Read configuration files received from the command line arguments '-c'
	if err := common.MergeConfigurationFiles("datadog", confPathArray, cmd.Flags().Lookup("cfgpath").Changed); err != nil {
		return err
	}

	client, err := secagent.NewRuntimeSecurityClient()
	if err != nil {
		return errors.Wrap(err, "unable to create a runtime security client instance")
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
		fmt.Println("Active dumps:")
		for _, d := range output.Dumps {
			printSecurityActivityDumpMessage("\t", d)
		}
	} else {
		fmt.Println("No active dumps found")
	}

	return nil
}

func stopActivityDump(cmd *cobra.Command, args []string) error {
	// Read configuration files received from the command line arguments '-c'
	if err := common.MergeConfigurationFiles("datadog", confPathArray, cmd.Flags().Lookup("cfgpath").Changed); err != nil {
		return err
	}

	client, err := secagent.NewRuntimeSecurityClient()
	if err != nil {
		return errors.Wrap(err, "unable to create a runtime security client instance")
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

func generateProfileFromActivityDump(cmd *cobra.Command, args []string) error {
	// Read configuration files received from the command line arguments '-c'
	if err := common.MergeConfigurationFiles("datadog", confPathArray, cmd.Flags().Lookup("cfgpath").Changed); err != nil {
		return err
	}

	var profilePath string

	if activityDumpArgs.remote {
		client, err := secagent.NewRuntimeSecurityClient()
		if err != nil {
			return fmt.Errorf("profile generation failed: %w", err)
		}
		defer client.Close()

		output, err := client.GenerateProfile(activityDumpArgs.file)
		if err != nil {
			return fmt.Errorf("couldn't send request to system-probe: %w", err)
		}
		if len(output.Error) > 0 {
			return fmt.Errorf("profile generation failed: %s", output.Error)
		}
		profilePath = output.ProfilePath
	} else {
		output, err := sprobe.GenerateProfile(activityDumpArgs.file)
		if err != nil {
			return fmt.Errorf("profile generation failed: %w", err)
		}
		profilePath = output
	}

	fmt.Printf("Generated profile: %s\n", profilePath)
	return nil
}

func generateGraphFromActivityDump(cmd *cobra.Command, args []string) error {
	// Read configuration files received from the command line arguments '-c'
	if err := common.MergeConfigurationFiles("datadog", confPathArray, cmd.Flags().Lookup("cfgpath").Changed); err != nil {
		return err
	}

	var graphPath string

	if activityDumpArgs.remote {
		client, err := secagent.NewRuntimeSecurityClient()
		if err != nil {
			return fmt.Errorf("graph generation failed: %w", err)
		}
		defer client.Close()

		output, err := client.GenerateGraph(activityDumpArgs.file)
		if err != nil {
			return fmt.Errorf("couldn't send request to system-probe: %w", err)
		}
		if len(output.Error) > 0 {
			return fmt.Errorf("graph generation failed: %s", output.Error)
		}
		graphPath = output.GraphPath
	} else {
		output, err := sprobe.GenerateGraph(activityDumpArgs.file)
		if err != nil {
			return fmt.Errorf("graph generation failed: %w", err)
		}
		graphPath = output
	}

	fmt.Printf("Generated graph: %s\n", graphPath)
	return nil
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

	var opts rules.Opts
	opts.
		WithConstants(model.SECLConstants).
		WithVariables(model.SECLVariables).
		WithSupportedDiscarders(sprobe.SupportedDiscarders).
		WithEventTypeEnabled(enabled).
		WithReservedRuleIDs(sprobe.AllCustomRuleIDs()).
		WithLegacyFields(model.SECLLegacyFields).
		WithLogger(&seclog.PatternLogger{})

	model := &model.Model{}
	ruleSet := rules.NewRuleSet(model, model.NewEvent, &opts)

	agentVersion, err := utils.GetAgentSemverVersion()
	if err != nil {
		return err
	}

	provider, err := rules.NewPoliciesDirProvider(cfg.PoliciesDir, false, agentVersion)
	if err != nil {
		return err
	}

	loader := rules.NewPolicyLoader(provider)

	if err := ruleSet.LoadPolicies(loader); err.ErrorOrNil() != nil {
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

func runRuntimeSelfTest(cmd *cobra.Command, args []string) error {
	client, err := secagent.NewRuntimeSecurityClient()
	if err != nil {
		return errors.Wrap(err, "unable to create a runtime security client instance")
	}
	defer client.Close()

	selfTestResult, err := client.RunSelfTest()
	if err != nil {
		return errors.Wrap(err, "unable to get a process self test")
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
		return errors.Wrap(err, "unable to create a runtime security client instance")
	}
	defer client.Close()

	_, err = client.ReloadPolicies()
	if err != nil {
		return errors.Wrap(err, "unable to reload policies")
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
		return nil, errors.Wrap(err, "unable to create a runtime security agent instance")
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
