// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package compliance

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/security-agent/app/common"
	"github.com/DataDog/datadog-agent/cmd/security-agent/app/subcommands/check"
	"github.com/DataDog/datadog-agent/comp/core"
	compconfig "github.com/DataDog/datadog-agent/comp/core/config"
	complog "github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/compliance/event"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
)

func Commands(globalParams *common.GlobalParams) []*cobra.Command {
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
	eventArgs := &eventCliParams{
		GlobalParams: globalParams,
	}

	eventCmd := &cobra.Command{
		Use:   "event",
		Short: "Issue logs to test Security Agent compliance events",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(eventRun,
				fx.Supply(eventArgs),
				fx.Supply(core.CreateBundleParams(
					"",
					core.WithSecurityAgentConfigFilePaths(globalParams.ConfPathArray),
					core.WithConfigLoadSecurityAgent(true),
				).LogForOneShot(common.LoggerName, "info", true)),
				core.Bundle,
			)
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

func eventRun(log complog.Component, config compconfig.Component, eventArgs *eventCliParams) error {
	stopper := startstop.NewSerialStopper()
	defer stopper.Stop()

	endpoints, dstContext, err := common.NewLogContextCompliance()
	if err != nil {
		return err
	}

	runPath := coreconfig.Datadog.GetString("compliance_config.run_path")
	reporter, err := event.NewLogReporter(stopper, eventArgs.sourceName, eventArgs.sourceType, runPath, endpoints, dstContext)
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
