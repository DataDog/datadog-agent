// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && kubeapiserver
// +build !windows,kubeapiserver

package app

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/DataDog/datadog-agent/cmd/security-agent/common"
	"github.com/DataDog/datadog-agent/pkg/compliance/agent"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks"
	"github.com/DataDog/datadog-agent/pkg/compliance/event"
	"github.com/DataDog/datadog-agent/pkg/config"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
	"github.com/cihub/seelog"
	"github.com/spf13/cobra"
)

var (
	checkArgs = struct {
		framework         string
		file              string
		verbose           bool
		report            bool
		overrideRegoInput string
		dumpRegoInput     string
		dumpReports       string
	}{}
)

func setupCheckCmd(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&checkArgs.framework, "framework", "", "", "Framework to run the checks from")
	cmd.Flags().StringVarP(&checkArgs.file, "file", "f", "", "Compliance suite file to read rules from")
	cmd.Flags().BoolVarP(&checkArgs.verbose, "verbose", "v", false, "Include verbose details")
	cmd.Flags().BoolVarP(&checkArgs.report, "report", "r", false, "Send report")
	cmd.Flags().StringVarP(&checkArgs.overrideRegoInput, "override-rego-input", "", "", "Rego input to use when running rego checks")
	cmd.Flags().StringVarP(&checkArgs.dumpRegoInput, "dump-rego-input", "", "", "Path to file where to dump the Rego input JSON")
	cmd.Flags().StringVarP(&checkArgs.dumpReports, "dump-reports", "", "", "Path to file where to dump reports")
}

// CheckCmd returns a cobra command to run security agent checks
func CheckCmd(confPathArrayGetter func() []string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Run compliance check(s)",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCheck(cmd, confPathArrayGetter(), args)
		},
	}
	setupCheckCmd(cmd)
	return cmd
}

func runCheck(cmd *cobra.Command, confPathArray []string, args []string) error {
	err := configureLogger()
	if err != nil {
		return err
	}

	// We need to set before calling `SetupConfig`
	configName := "datadog"
	if flavor.GetFlavor() == flavor.ClusterAgent {
		configName = "datadog-cluster"
	}

	// Read configuration files received from the command line arguments '-c'
	if err := common.MergeConfigurationFiles(configName, confPathArray, cmd.Flags().Lookup("cfgpath").Changed); err != nil {
		return err
	}

	options := []checks.BuilderOption{}

	if flavor.GetFlavor() == flavor.ClusterAgent {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		log.Info("Waiting for APIClient")
		apiCl, err := apiserver.WaitForAPIClient(ctx)
		if err != nil {
			return err
		}
		options = append(options, checks.MayFail(checks.WithKubernetesClient(apiCl.DynamicCl, "")))
	} else {
		options = append(options, []checks.BuilderOption{
			checks.WithHostRootMount(os.Getenv("HOST_ROOT")),
			checks.MayFail(checks.WithDocker()),
			checks.MayFail(checks.WithAudit()),
		}...)

		if config.IsKubernetes() {
			nodeLabels, err := agent.WaitGetNodeLabels()
			if err != nil {
				log.Error(err)
			} else {
				options = append(options, checks.WithNodeLabels(nodeLabels))
			}
		}
	}

	var ruleID string
	if len(args) != 0 {
		ruleID = args[0]
	}

	hostname, err := util.GetHostname(context.TODO())
	if err != nil {
		return err
	}

	options = append(options, checks.WithHostname(hostname))

	stopper = startstop.NewSerialStopper()
	defer stopper.Stop()

	reporter, err := NewCheckReporter(stopper, checkArgs.report, checkArgs.dumpReports)
	if err != nil {
		return err
	}

	if ruleID != "" {
		log.Infof("Looking for rule with ID=%s", ruleID)
		options = append(options, checks.WithMatchRule(checks.IsRuleID(ruleID)))
	}

	if checkArgs.framework != "" {
		log.Infof("Looking for rules with framework=%s", checkArgs.framework)
		options = append(options, checks.WithMatchSuite(checks.IsFramework(checkArgs.framework)))
	}

	if checkArgs.overrideRegoInput != "" {
		log.Infof("Running on provided rego input: path=%s", checkArgs.overrideRegoInput)
		options = append(options, checks.WithRegoInput(checkArgs.overrideRegoInput))
	}

	if checkArgs.dumpRegoInput != "" {
		options = append(options, checks.WithRegoInputDumpPath(checkArgs.dumpRegoInput))
	}

	if checkArgs.file != "" {
		err = agent.RunChecksFromFile(reporter, checkArgs.file, options...)
	} else {
		configDir := config.Datadog.GetString("compliance_config.dir")
		err = agent.RunChecks(reporter, configDir, options...)
	}

	if err != nil {
		log.Errorf("Failed to run checks: %v", err)
		return err
	}

	if err := reporter.dumpReports(); err != nil {
		log.Errorf("Failed to dump reports %v", err)
		return err
	}

	return nil
}

func configureLogger() error {
	var (
		logFormat = "%LEVEL | %Msg%n"
		logLevel  = "info"
	)
	if checkArgs.verbose {
		const logDateFormat = "2006-01-02 15:04:05 MST"
		logFormat = fmt.Sprintf("%%Date(%s) | %%LEVEL | (%%ShortFilePath:%%Line in %%FuncShort) | %%Msg%%n", logDateFormat)
		logLevel = "trace"
	}
	logger, err := seelog.LoggerFromWriterWithMinLevelAndFormat(os.Stdout, seelog.DebugLvl, logFormat)
	if err != nil {
		return err
	}

	log.SetupLogger(logger, logLevel)
	return nil
}

type RunCheckReporter struct {
	reporter        event.Reporter
	events          map[string][]*event.Event
	dumpReportsPath string
}

func NewCheckReporter(stopper startstop.Stopper, report bool, dumpReportsPath string) (*RunCheckReporter, error) {
	r := &RunCheckReporter{}

	if report {
		endpoints, dstContext, err := newLogContextCompliance()
		if err != nil {
			return nil, err
		}

		runPath := coreconfig.Datadog.GetString("compliance_config.run_path")
		reporter, err := event.NewLogReporter(stopper, eventArgs.sourceName, eventArgs.sourceType, runPath, endpoints, dstContext)
		if err != nil {
			return nil, fmt.Errorf("failed to set up compliance log reporter: %w", err)
		}

		r.reporter = reporter
	}

	r.events = make(map[string][]*event.Event)
	r.dumpReportsPath = dumpReportsPath

	return r, nil
}

func (r *RunCheckReporter) Report(event *event.Event) {
	r.events[event.AgentRuleID] = append(r.events[event.AgentRuleID], event)

	eventJSON, err := checks.PrettyPrintJSON(event, "  ")
	if err != nil {
		log.Errorf("Failed to marshal rule event: %v", err)
		return
	}

	r.ReportRaw(eventJSON, "")

	if r.reporter != nil {
		r.reporter.Report(event)
	}
}

func (r *RunCheckReporter) ReportRaw(content []byte, service string, tags ...string) {
	fmt.Println(string(content))
}

func (r *RunCheckReporter) dumpReports() error {
	if r.dumpReportsPath != "" {
		reportsJSON, err := checks.PrettyPrintJSON(r.events, "\t")
		if err != nil {
			return err
		}

		return os.WriteFile(r.dumpReportsPath, reportsJSON, 0644)
	}
	return nil
}

func init() {
	complianceCmd.AddCommand(CheckCmd(func() []string {
		return confPathArray
	}))
}
