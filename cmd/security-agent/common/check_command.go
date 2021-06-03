// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build kubeapiserver

package common

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/DataDog/datadog-agent/pkg/compliance/agent"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks"
	"github.com/DataDog/datadog-agent/pkg/compliance/event"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/cihub/seelog"

	"github.com/spf13/cobra"
)

var (
	checkArgs = struct {
		framework string
		file      string
		verbose   bool
	}{}
)

func setupCheckCmd(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&checkArgs.framework, "framework", "", "", "Framework to run the checks from")
	cmd.Flags().StringVarP(&checkArgs.file, "file", "f", "", "Compliance suite file to read rules from")
	cmd.Flags().BoolVarP(&checkArgs.verbose, "verbose", "v", false, "Include verbose details")
}

// CheckCmd returns a cobra command to run security agent checks
func CheckCmd(confPathArray []string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check [rule ID]",
		Short: "Run compliance check(s)",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCheck(cmd, confPathArray, args)
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
	if err := MergeConfigurationFiles(configName, confPathArray, cmd.Flags().Lookup("cfgpath").Changed); err != nil {
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

	hostname, err := util.GetHostname()
	if err != nil {
		return err
	}

	options = append(options, checks.WithHostname(hostname))

	reporter := &runCheckReporter{}

	if ruleID != "" {
		log.Infof("Looking for rule with ID=%s", ruleID)
		options = append(options, checks.WithMatchRule(checks.IsRuleID(ruleID)))
	}

	if checkArgs.framework != "" {
		log.Infof("Looking for rules with framework=%s", checkArgs.framework)
		options = append(options, checks.WithMatchSuite(checks.IsFramework(checkArgs.framework)))
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

type runCheckReporter struct {
}

func (r *runCheckReporter) Report(event *event.Event) {
	data, err := json.Marshal(event)
	if err != nil {
		log.Errorf("Failed to marshal rule event: %v", err)
		return
	}

	var buf bytes.Buffer
	_ = json.Indent(&buf, data, "", "  ")
	r.ReportRaw(buf.Bytes())
}

func (r *runCheckReporter) ReportRaw(content []byte, tags ...string) {
	fmt.Println(string(content))
}
