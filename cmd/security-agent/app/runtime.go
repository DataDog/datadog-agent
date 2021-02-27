// +build linux

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package app

import (
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/pkg/compliance/event"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/restart"
	secagent "github.com/DataDog/datadog-agent/pkg/security/agent"
	secconfig "github.com/DataDog/datadog-agent/pkg/security/config"
	securityLogger "github.com/DataDog/datadog-agent/pkg/security/log"
	"github.com/DataDog/datadog-agent/pkg/security/model"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	ddgostatsd "github.com/DataDog/datadog-go/statsd"
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
		dir string
	}{}

	dumpCmd = &cobra.Command{
		Use:   "dump",
		Short: "Dump security module information",
	}

	dumpProcessCacheCmd = &cobra.Command{
		Use:   "process-cache",
		Short: "process cache",
		RunE:  dumpProcessCache,
	}
)

func init() {
	dumpCmd.AddCommand(dumpProcessCacheCmd)
	runtimeCmd.AddCommand(dumpCmd)

	runtimeCmd.AddCommand(checkPoliciesCmd)
	checkPoliciesCmd.Flags().StringVar(&checkPoliciesArgs.dir, "policies-dir", coreconfig.DefaultRuntimePoliciesDir, "Path to policies directory")
}

func dumpProcessCache(cmd *cobra.Command, args []string) error {
	client, err := secagent.NewRuntimeSecurityClient()
	if err != nil {
		return errors.Wrap(err, "unable to create a runtime security client instance")
	}
	defer client.Close()

	filename, err := client.DumpProcessCache()
	if err != nil {
		return errors.Wrap(err, "unable to get a process cache dump")
	}

	fmt.Printf("Dump written: %s\n", filename)

	return nil
}

func checkPolicies(cmd *cobra.Command, args []string) error {
	cfg := &secconfig.Config{
		PoliciesDir:         checkPoliciesArgs.dir,
		EnableKernelFilters: true,
		EnableApprovers:     true,
		EnableDiscarders:    true,
		PIDCacheSize:        1,
	}

	// enabled all the rules
	enabled := map[eval.EventType]bool{"*": true}

	opts := rules.NewOptsWithParams(model.SECLConstants, sprobe.SupportedDiscarders, enabled, sprobe.AllCustomRuleIDs(), model.SECLLegacyAttributes, securityLogger.DatadogAgentLogger{})
	model := &sprobe.Model{}
	ruleSet := rules.NewRuleSet(model, model.NewEvent, opts)

	if err := rules.LoadPolicies(cfg.PoliciesDir, ruleSet); err != nil {
		return err
	}

	rsa := sprobe.NewRuleSetApplier(cfg, nil)

	report, err := rsa.Apply(ruleSet)
	if err != nil {
		return err
	}

	content, _ := json.MarshalIndent(report, "", "\t")
	fmt.Printf("%s\n", string(content))

	return nil
}

func newRuntimeReporter(stopper restart.Stopper, sourceName, sourceType string, endpoints *config.Endpoints, context *client.DestinationsContext) (event.Reporter, error) {
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
			Type:    sourceType,
			Service: sourceName,
			Source:  sourceName,
		},
	)
	return event.NewReporter(logSource, pipelineProvider.NextPipelineChan()), nil
}

func startRuntimeSecurity(hostname string, stopper restart.Stopper, statsdClient *ddgostatsd.Client) (*secagent.RuntimeSecurityAgent, error) {
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

	agent, err := secagent.NewRuntimeSecurityAgent(hostname, reporter)
	if err != nil {
		return nil, errors.Wrap(err, "unable to create a runtime security agent instance")
	}
	agent.Start()

	stopper.Add(agent)

	log.Info("Datadog runtime security agent is now running")

	return agent, nil
}
