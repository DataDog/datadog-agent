// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package app

import (
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/security-agent/app/common"
	"github.com/DataDog/datadog-agent/cmd/security-agent/app/subcommands/check"
	"github.com/DataDog/datadog-agent/pkg/compliance/event"
)

func ComplianceCommands(globalParams *common.GlobalParams) []*cobra.Command {
	complianceCmd := &cobra.Command{
		Use:   "compliance",
		Short: "Compliance Agent utility commands",
	}

	complianceCmd.AddCommand(complianceEventCommand(globalParams))
	complianceCmd.AddCommand(check.SecAgentCommands(globalParams)...)

	return []*cobra.Command{complianceCmd}
}

type eventCliParams struct {
	*common.GlobalParams

	sourceName string
	sourceType string
	event      event.Event
	data       []string
}

func complianceEventCommand(globalParams *common.GlobalParams) *cobra.Command {
	eventArgs := eventCliParams{
		GlobalParams: globalParams,
	}

	eventCmd := &cobra.Command{
		Use:   "event",
		Short: "Issue logs to test Security Agent compliance events",
		RunE: func(cmd *cobra.Command, args []string) error {
			return eventRun(&eventArgs)
		},
		Hidden: true,
	}

	eventCmd.Flags().StringVarP(&eventArgs.sourceType, "source-type", "", "compliance", "Log source name")
	eventCmd.Flags().StringVarP(&eventArgs.sourceName, "source-name", "", "compliance-agent", "Log source name")
	eventCmd.Flags().StringVarP(&eventArgs.event.AgentRuleID, "rule-id", "", "", "Rule ID")
	eventCmd.Flags().StringVarP(&eventArgs.event.ResourceID, "resource-id", "", "", "Resource ID")
	eventCmd.Flags().StringVarP(&eventArgs.event.ResourceType, "resource-type", "", "", "Resource type")
	eventCmd.Flags().StringSliceVarP(&eventArgs.event.Tags, "tags", "t", []string{"security:compliance"}, "Tags")
	eventCmd.Flags().StringSliceVarP(&eventArgs.data, "data", "d", []string{}, "Data KV fields")

	return eventCmd
}
