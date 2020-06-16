// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.
// +build kubeapiserver

package app

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/compliance"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/logs/restart"
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
		sourceName string
		sourceType string
		event      compliance.RuleEvent
		data       []string
	}{}
)

func init() {
	complianceCmd.AddCommand(eventCmd)
	eventCmd.Flags().StringVarP(&eventArgs.sourceType, "source-type", "", "compliance", "Log source name")
	eventCmd.Flags().StringVarP(&eventArgs.sourceName, "source-name", "", "compliance-agent", "Log source name")
	eventCmd.Flags().StringVarP(&eventArgs.event.RuleID, "rule-id", "", "", "Rule ID")
	eventCmd.Flags().StringVarP(&eventArgs.event.Version, "version", "", "", "Framework version")
	eventCmd.Flags().StringVarP(&eventArgs.event.Framework, "framework", "", "", "Compliance framework")
	eventCmd.Flags().StringVarP(&eventArgs.event.ResourceID, "resource-id", "", "", "Resource ID")
	eventCmd.Flags().StringVarP(&eventArgs.event.ResourceType, "resource-type", "", "", "Resource type")
	eventCmd.Flags().StringSliceVarP(&eventArgs.event.Tags, "tags", "t", []string{"security:compliance"}, "Tags")
	eventCmd.Flags().StringSliceVarP(&eventArgs.data, "data", "d", []string{}, "Data KV fields")
}

func event(cmd *cobra.Command, args []string) error {
	// we'll search for a config file named `datadog.yaml`
	coreconfig.Datadog.SetConfigName("datadog")
	err := common.SetupConfig(confPath)
	if err != nil {
		return fmt.Errorf("unable to set up global agent configuration: %w", err)
	}

	stopper := restart.NewSerialStopper()
	defer stopper.Stop()

	reporter, err := newComplianceReporter(stopper, eventArgs.sourceName, eventArgs.sourceType)
	if err != nil {
		return fmt.Errorf("failed to set up compliance log reporter: %w", err)
	}

	eventArgs.event.Data = map[string]string{}
	for _, d := range eventArgs.data {
		kv := strings.SplitN(d, ":", 2)
		if len(kv) != 2 {
			continue
		}
		eventArgs.event.Data[kv[0]] = kv[1]
	}

	reporter.Report(&eventArgs.event)

	return nil
}
