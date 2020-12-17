// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package app

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	secagentcommon "github.com/DataDog/datadog-agent/cmd/security-agent/common"
	"github.com/DataDog/datadog-agent/pkg/collector/runner"
	"github.com/DataDog/datadog-agent/pkg/collector/scheduler"
	"github.com/DataDog/datadog-agent/pkg/compliance/agent"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks"
	"github.com/DataDog/datadog-agent/pkg/compliance/event"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/restart"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	ddgostatsd "github.com/DataDog/datadog-go/statsd"
)

var (
	complianceCmd = &cobra.Command{
		Use:   "compliance",
		Short: "Compliance Agent utility commands",
	}

	eventCmd = &cobra.Command{
		Use:   "event",
		Short: "Issue logs to test Security Agent compliance events",
		RunE:  eventRun,
	}

	eventArgs = struct {
		sourceName string
		sourceType string
		event      event.Event
		data       []string
	}{}
)

func init() {
	complianceCmd.AddCommand(eventCmd)
	eventCmd.Flags().StringVarP(&eventArgs.sourceType, "source-type", "", "compliance", "Log source name")
	eventCmd.Flags().StringVarP(&eventArgs.sourceName, "source-name", "", "compliance-agent", "Log source name")
	eventCmd.Flags().StringVarP(&eventArgs.event.AgentRuleID, "rule-id", "", "", "Rule ID")
	eventCmd.Flags().StringVarP(&eventArgs.event.ResourceID, "resource-id", "", "", "Resource ID")
	eventCmd.Flags().StringVarP(&eventArgs.event.ResourceType, "resource-type", "", "", "Resource type")
	eventCmd.Flags().StringSliceVarP(&eventArgs.event.Tags, "tags", "t", []string{"security:compliance"}, "Tags")
	eventCmd.Flags().StringSliceVarP(&eventArgs.data, "data", "d", []string{}, "Data KV fields")
}

func eventRun(cmd *cobra.Command, args []string) error {
	// Read configuration files received from the command line arguments '-c'
	if err := secagentcommon.MergeConfigurationFiles("datadog", confPathArray, cmd.Flags().Lookup("cfgpath").Changed); err != nil {
		return err
	}

	stopper := restart.NewSerialStopper()
	defer stopper.Stop()

	endpoints, dstContext, err := newLogContextCompliance()
	if err != nil {
		return err
	}

	reporter, err := newComplianceReporter(stopper, eventArgs.sourceName, eventArgs.sourceType, endpoints, dstContext)
	if err != nil {
		return fmt.Errorf("failed to set up compliance log reporter: %w", err)
	}

	eventData := event.Data{}
	for _, d := range eventArgs.data {
		kv := strings.SplitN(d, ":", 2)
		if len(kv) != 2 {
			continue
		}
		eventData[kv[0]] = kv[1]
	}
	eventArgs.event.Data = eventData

	reporter.Report(&eventArgs.event)

	return nil
}

func newComplianceReporter(stopper restart.Stopper, sourceName, sourceType string, endpoints *config.Endpoints, context *client.DestinationsContext) (event.Reporter, error) {
	health := health.RegisterLiveness("compliance")

	// setup the auditor
	auditor := auditor.New(coreconfig.Datadog.GetString("compliance_config.run_path"), "compliance-registry.json", health)
	auditor.Start()
	stopper.Add(auditor)

	// setup the pipeline provider that provides pairs of processor and sender
	pipelineProvider := pipeline.NewProvider(config.NumberOfPipelines, auditor, nil, endpoints, context)
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

func startCompliance(hostname string, stopper restart.Stopper, statsdClient *ddgostatsd.Client) error {
	enabled := coreconfig.Datadog.GetBool("compliance_config.enabled")
	if !enabled {
		return nil
	}

	endpoints, context, err := newLogContextCompliance()
	if err != nil {
		log.Error(err)
	}
	stopper.Add(context)

	reporter, err := newComplianceReporter(stopper, "compliance-agent", "compliance", endpoints, context)
	if err != nil {
		return err
	}

	runner := runner.NewRunner()
	stopper.Add(runner)

	scheduler := scheduler.NewScheduler(runner.GetChan())
	runner.SetScheduler(scheduler)

	checkInterval := coreconfig.Datadog.GetDuration("compliance_config.check_interval")
	configDir := coreconfig.Datadog.GetString("compliance_config.dir")

	options := []checks.BuilderOption{
		checks.WithInterval(checkInterval),
		checks.WithHostname(hostname),
		checks.WithHostRootMount(os.Getenv("HOST_ROOT")),
		checks.MayFail(checks.WithDocker()),
		checks.MayFail(checks.WithAudit()),
	}

	if coreconfig.IsKubernetes() {
		nodeLabels, err := agent.WaitGetNodeLabels()
		if err != nil {
			log.Error(err)
		} else {
			options = append(options, checks.WithNodeLabels(nodeLabels))
		}
	}

	agent, err := agent.New(
		reporter,
		scheduler,
		configDir,
		options...,
	)
	if err != nil {
		log.Errorf("Compliance agent failed to initialize: %v", err)
		return err
	}
	err = agent.Run()
	if err != nil {
		log.Errorf("Error starting compliance agent, exiting: %v", err)
		return err
	}
	stopper.Add(agent)

	log.Infof("Running compliance checks every %s", checkInterval.String())

	// Send the compliance 'running' metrics periodically
	ticker := sendRunningMetrics(statsdClient, "compliance")
	stopper.Add(ticker)

	return nil
}
