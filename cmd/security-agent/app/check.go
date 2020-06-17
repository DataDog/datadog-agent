// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/agent"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks"
	"github.com/DataDog/datadog-agent/pkg/config"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/spf13/cobra"
)

var (
	checkCmd = &cobra.Command{
		Use:   "check [rule ID]",
		Short: "Run compliance check(s)",
		Long:  ``,
		RunE:  runCheck,
	}

	checkArgs = struct {
		framework string
		file      string
	}{}
)

func init() {
	SecurityAgentCmd.AddCommand(checkCmd)
	checkCmd.Flags().StringVarP(&checkArgs.framework, "framework", "", "", "Framework to run the checks from")
	checkCmd.Flags().StringVarP(&checkArgs.file, "file", "f", "", "Compliance suite file to read rules from")
}

func runCheck(cmd *cobra.Command, args []string) error {
	// we'll search for a config file named `datadog.yaml`
	config.Datadog.SetConfigName("datadog")
	err := common.SetupConfig(confPath)
	if err != nil {
		return fmt.Errorf("unable to set up global security agent configuration: %v", err)
	}

	var ruleID string
	if len(args) != 0 {
		ruleID = args[0]
	}

	hostname, err := util.GetHostname()
	if err != nil {
		return err
	}

	reporter := &runCheckReporter{}

	options := []checks.BuilderOption{
		checks.WithHostname(hostname),
		checks.WithHostRootMount(os.Getenv("HOST_ROOT")),
		checks.MayFail(checks.WithDocker()),
		checks.MayFail(checks.WithAudit()),
	}

	if ruleID != "" {
		options = append(options, checks.WithMatchRule(checks.IsRuleID(ruleID)))
	}

	if checkArgs.framework != "" {
		options = append(options, checks.WithMatchSuite(checks.IsFramework(checkArgs.framework)))
	}

	if checkArgs.file != "" {
		err = agent.RunChecksFromFile(reporter, checkArgs.file, options...)
	} else {
		configDir := coreconfig.Datadog.GetString("compliance_config.dir")
		err = agent.RunChecks(reporter, configDir, options...)
	}

	if err != nil {
		log.Errorf("Failed to run checks: %v", err)
		return err
	}
	return nil
}

type runCheckReporter struct {
}

func (r *runCheckReporter) Report(event *compliance.RuleEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		log.Errorf("Failed to marshal rule event: %v", err)
		return
	}

	var buf bytes.Buffer
	_ = json.Indent(&buf, data, "", "  ")

	fmt.Println(buf.String())
}
