// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package common

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/compliance/agent"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks"
	"github.com/DataDog/datadog-agent/pkg/compliance/event"
	"github.com/DataDog/datadog-agent/pkg/config"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/spf13/cobra"
)

var (
	checkArgs = struct {
		framework string
		file      string
	}{}
)

func setupCheckCmd(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&checkArgs.framework, "framework", "", "", "Framework to run the checks from")
	cmd.Flags().StringVarP(&checkArgs.file, "file", "f", "", "Compliance suite file to read rules from")
}

// CheckCmd returns a cobra command to run security agent checks
func CheckCmd(confPath *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check [rule ID]",
		Short: "Run compliance check(s)",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCheck(cmd, confPath, args)
		},
	}
	setupCheckCmd(cmd)
	return cmd
}

func runCheck(cmd *cobra.Command, confPath *string, args []string) error {
	options := []checks.BuilderOption{}
	if flavor.GetFlavor() == flavor.ClusterAgent {
		config.Datadog.SetConfigName("datadog-cluster")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		apiCl, err := apiserver.WaitForAPIClient(ctx)
		if err != nil {
			return err
		}
		options = append(options, checks.MayFail(checks.WithKubernetesClient(apiCl.DynamicCl)))
	} else {
		config.Datadog.SetConfigName("datadog")
		options = append(options, []checks.BuilderOption{
			checks.WithHostRootMount(os.Getenv("HOST_ROOT")),
			checks.MayFail(checks.WithDocker()),
			checks.MayFail(checks.WithAudit()),
		}...)
	}

	err := common.SetupConfig(*confPath)
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

	options = append(options, checks.WithHostname(hostname))

	reporter := &runCheckReporter{}

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

func (r *runCheckReporter) Report(event *event.Event) {
	data, err := json.Marshal(event)
	if err != nil {
		log.Errorf("Failed to marshal rule event: %v", err)
		return
	}

	var buf bytes.Buffer
	_ = json.Indent(&buf, data, "", "  ")

	fmt.Println(buf.String())
}
