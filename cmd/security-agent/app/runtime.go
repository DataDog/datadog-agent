// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package app

import (
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/logs/restart"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	secagent "github.com/DataDog/datadog-agent/pkg/security/agent"
	secmodule "github.com/DataDog/datadog-agent/pkg/security/module"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	runtimeCmd = &cobra.Command{
		Use:   "runtime",
		Short: "Runtime Agent utility commands",
	}

	checkPoliciesCmd = &cobra.Command{
		Use:   "check-policies",
		Short: "Check policies and return a report",
		RunE:  checkPolicies,
	}

	checkPoliciesArgs = struct {
		configPath string
	}{}
)

func init() {
	runtimeCmd.AddCommand(checkPoliciesCmd)
	checkPoliciesCmd.Flags().StringVar(&checkPoliciesArgs.configPath, "config", "/etc/datadog-agent/system-probe.yaml", "Path to system-probe config formatted as YAML")
}

func checkPolicies(cmd *cobra.Command, args []string) error {
	cfg, err := config.NewSystemProbeConfig(loggerName, checkPoliciesArgs.configPath)
	if err != nil {
		return err
	}

	module, err := secmodule.NewModule(cfg)
	if err != nil {
		return err
	}

	secModule := module.(*secmodule.Module)
	report, err := secModule.ApplyRuleSet(true)
	if err != nil {
		return err
	}

	content, _ := json.MarshalIndent(report, "", "\t")
	fmt.Printf("%s\n", string(content))

	return nil
}

func startRuntimeSecurity(stopper restart.Stopper) error {
	enabled := coreconfig.Datadog.GetBool("runtime_security_config.enabled")
	if !enabled {
		log.Info("Datadog runtime security agent disabled by config")
		return nil
	}

	agent, err := secagent.NewRuntimeSecurityAgent()
	if err != nil {
		return errors.Wrap(err, "unable to create a runtime security agent instance")
	}
	agent.Start()

	stopper.Add(agent)

	log.Info("Datadog runtime security agent is now running")

	return nil
}
