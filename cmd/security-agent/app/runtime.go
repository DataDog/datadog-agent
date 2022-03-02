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
	"time"

	ddgostatsd "github.com/DataDog/datadog-go/statsd"
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
	secagent "github.com/DataDog/datadog-agent/pkg/security/agent"
	secconfig "github.com/DataDog/datadog-agent/pkg/security/config"
	seclog "github.com/DataDog/datadog-agent/pkg/security/log"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
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
		tags              []string
		comm              string
		file              string
		timeout           int
		withGraph         bool
		differentiateArgs bool
	}{}

	activityDumpGenerateCmd = &cobra.Command{
		Use:   "generate",
		Short: "generate an activity dump",
		RunE:  generateActivityDump,
	}

	activityDumpGenerateProfileCmd = &cobra.Command{
		Use:   "generate-profile",
		Short: "generate a profile from an activity dump",
		RunE:  generateProfileFromActivityDump,
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

	activityDumpGenerateCmd.Flags().StringArrayVar(
		&activityDumpArgs.tags,
		"tags",
		[]string{},
		"tags are used to filter the activity dump in order to select a specific workload. Tags should be provided in the \"tag_name:tag_value\" format.",
	)
	activityDumpGenerateCmd.Flags().StringVar(
		&activityDumpArgs.comm,
		"comm",
		"",
		"a process command can be used to filter the activity dump from a specific process.",
	)
	activityDumpGenerateCmd.Flags().IntVar(
		&activityDumpArgs.timeout,
		"timeout",
		10,
		"timeout for the activity dump in minutes",
	)
	activityDumpGenerateCmd.Flags().BoolVar(
		&activityDumpArgs.withGraph,
		"graph",
		false,
		"generate a graph from the generated dump",
	)
	activityDumpGenerateCmd.Flags().BoolVar(
		&activityDumpArgs.differentiateArgs,
		"differentiate-args",
		false,
		"add the arguments in the process node merge algorithm",
	)

	activityDumpStopCmd.Flags().StringArrayVar(
		&activityDumpArgs.tags,
		"tags",
		[]string{},
		"tags is used to select an activity dump. Tags should be provided in the [tag_name:tag_value] format.",
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
		"path to the activity dump file from which a profile will be generated",
	)
	_ = activityDumpGenerateProfileCmd.MarkFlagRequired("input")

	processCacheCmd.AddCommand(processCacheDumpCmd)
	runtimeCmd.AddCommand(processCacheCmd)

	activityDumpCmd.AddCommand(activityDumpGenerateCmd)
	activityDumpCmd.AddCommand(activityDumpListCmd)
	activityDumpCmd.AddCommand(activityDumpStopCmd)
	activityDumpCmd.AddCommand(activityDumpGenerateProfileCmd)
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

func generateActivityDump(cmd *cobra.Command, args []string) error {
	// Read configuration files received from the command line arguments '-c'
	if err := common.MergeConfigurationFiles("datadog", confPathArray, cmd.Flags().Lookup("cfgpath").Changed); err != nil {
		return err
	}

	client, err := secagent.NewRuntimeSecurityClient()
	if err != nil {
		return errors.Wrap(err, "unable to create a runtime security client instance")
	}
	defer client.Close()

	var filename, graph string
	filename, graph, err = client.GenerateActivityDump(activityDumpArgs.tags, activityDumpArgs.comm, int32(activityDumpArgs.timeout), activityDumpArgs.withGraph, activityDumpArgs.differentiateArgs)
	if err != nil {
		return errors.Wrap(err, "unable to an request activity dump for %s")
	}

	fmt.Printf("Activity dump file: %s\n", filename)
	if len(graph) > 0 {
		fmt.Printf("Graph dump file: %s\n", graph)
	}

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

	var activeDumps []string
	activeDumps, err = client.ListActivityDumps()
	if err != nil {
		return errors.Wrap(err, "unable to request the list activity dumps")
	}

	if len(activeDumps) > 0 {
		fmt.Println("Active dumps:")
		for _, d := range activeDumps {
			fmt.Printf("\t- %s\n", d)
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

	var msg string
	msg, err = client.StopActivityDump(activityDumpArgs.tags, activityDumpArgs.comm)
	if err != nil {
		return errors.Wrap(err, "unable to stop the request activity dump")
	}

	if len(msg) == 0 {
		fmt.Println("done!")
	} else {
		fmt.Println(msg)
	}

	return nil
}

func generateProfileFromActivityDump(cmd *cobra.Command, args []string) error {
	// Read configuration files received from the command line arguments '-c'
	if err := common.MergeConfigurationFiles("datadog", confPathArray, cmd.Flags().Lookup("cfgpath").Changed); err != nil {
		return err
	}

	client, err := secagent.NewRuntimeSecurityClient()
	if err != nil {
		return errors.Wrap(err, "unable to generate a profile")
	}
	defer client.Close()

	var output string
	output, err = client.GenerateProfile(activityDumpArgs.file)
	if err != nil {
		return errors.Wrapf(err, "couldn't generate a profile from: %s", activityDumpArgs.file)
	}

	fmt.Printf("Generated profile: %s\n", output)
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

	if err := rules.LoadPolicies(cfg.PoliciesDir, ruleSet); err.ErrorOrNil() != nil {
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

	logSource := config.NewLogSource(
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

	endpoints, context, err := newLogContextRuntime()
	if err != nil {
		log.Error(err)
	}
	stopper.Add(context)

	reporter, err := newRuntimeReporter(stopper, "runtime-security-agent", "runtime-security", endpoints, context)
	if err != nil {
		return nil, err
	}

	agent, err := secagent.NewRuntimeSecurityAgent(hostname, reporter, endpoints)
	if err != nil {
		return nil, errors.Wrap(err, "unable to create a runtime security agent instance")
	}
	agent.Start()

	stopper.Add(agent)

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
