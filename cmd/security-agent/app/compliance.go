// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.
// +build kubeapiserver

package app

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/compliance"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/client/http"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/status/health"
)

var (
	complianceCmd = &cobra.Command{
		Use:   "compliance",
		Short: "Compliance Agent utility commands",
	}

	eventCmd = &cobra.Command{
		Use:   "event",
		Short: "Issue logs to test Security Agent compliance events",
		RunE:  event,
	}

	eventArgs = struct {
		tags          []string
		sourceName    string
		sourceType    string
		sourceService string
		event         compliance.RuleEvent
		data          []string
	}{}
)

func init() {
	complianceCmd.AddCommand(eventCmd)
	eventCmd.Flags().StringSliceVarP(&eventArgs.tags, "tags", "t", []string{"security:compliance"}, "Tags")
	eventCmd.Flags().StringVarP(&eventArgs.sourceType, "source-type", "", "compliance", "Log source name")
	eventCmd.Flags().StringVarP(&eventArgs.sourceName, "source-name", "", "compliance-agent", "Log source name")
	eventCmd.Flags().StringVarP(&eventArgs.sourceService, "source-service", "", "compliance-agent", "Log source service")
	eventCmd.Flags().StringVarP(&eventArgs.event.RuleName, "rule-name", "", "", "Rule name")
	eventCmd.Flags().StringVarP(&eventArgs.event.RuleVersion, "rule-version", "", "", "Rule version")
	eventCmd.Flags().StringVarP(&eventArgs.event.Framework, "framework", "", "", "Compliance framework")
	eventCmd.Flags().StringVarP(&eventArgs.event.ResourceID, "resource-id", "", "", "Resource ID")
	eventCmd.Flags().StringVarP(&eventArgs.event.ResourceType, "resource-type", "", "", "Resource type")
	eventCmd.Flags().StringSliceVarP(&eventArgs.data, "data", "d", []string{}, "Data KV fields")
}

func event(cmd *cobra.Command, args []string) error {
	// we'll search for a config file named `datadog-security.yaml`
	coreconfig.Datadog.SetConfigName("datadog-security")
	err := common.SetupConfig(confPath)
	if err != nil {
		return fmt.Errorf("unable to set up global agent configuration: %v", err)
	}

	httpConnectivity := config.HTTPConnectivityFailure
	if endpoints, err := config.BuildHTTPEndpoints(); err == nil {
		httpConnectivity = http.CheckConnectivity(endpoints.Main)
	}
	endpoints, err := config.BuildEndpoints(httpConnectivity)
	if err != nil {
		return fmt.Errorf("Invalid endpoints: %v", err)
	}

	destinationsCtx := client.NewDestinationsContext()
	destinationsCtx.Start()
	defer destinationsCtx.Stop()

	health := health.RegisterLiveness("security-agent")

	// setup the auditor
	auditor := auditor.New(coreconfig.Datadog.GetString("compliance_config.run_path"), health)
	auditor.Start()
	defer auditor.Stop()

	// setup the pipeline provider that provides pairs of processor and sender
	pipelineProvider := pipeline.NewProvider(config.NumberOfPipelines, auditor, nil, endpoints, destinationsCtx)
	pipelineProvider.Start()
	defer pipelineProvider.Stop()

	src := config.NewLogSource("compliance-agent", &config.LogsConfig{
		Type:    eventArgs.sourceType,
		Service: eventArgs.sourceService,
		Source:  eventArgs.sourceName,
		Tags:    eventArgs.tags,
	})

	eventArgs.event.Data = map[string]string{}
	for _, d := range eventArgs.data {
		kv := strings.SplitN(d, ":", 2)
		if len(kv) != 2 {
			continue
		}
		eventArgs.event.Data[kv[0]] = kv[1]
	}
	buf, err := json.Marshal(eventArgs.event)
	if err != nil {
		return err
	}

	msg := message.NewMessageWithSource(buf, message.StatusInfo, src)

	ch := pipelineProvider.NextPipelineChan()
	ch <- msg

	return nil
}
