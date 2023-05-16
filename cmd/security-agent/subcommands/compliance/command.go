// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package compliance

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/security-agent/command"
	"github.com/DataDog/datadog-agent/cmd/security-agent/flags"
	"github.com/DataDog/datadog-agent/cmd/security-agent/subcommands/check"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
)

func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	complianceCmd := &cobra.Command{
		Use:   "compliance",
		Short: "Compliance Agent utility commands",
	}

	complianceCmd.AddCommand(check.SecurityAgentCommands(globalParams)...)
	complianceCmd.AddCommand(complianceEventCommand(globalParams))

	return []*cobra.Command{complianceCmd}
}

type cliParams struct {
	*command.GlobalParams

	sourceName string
	sourceType string
	event      compliance.CheckEvent
	data       []string
}

func complianceEventCommand(globalParams *command.GlobalParams) *cobra.Command {
	eventArgs := &cliParams{
		GlobalParams: globalParams,
	}

	eventCmd := &cobra.Command{
		Use:   "event",
		Short: "Issue logs to test Security Agent compliance events",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(eventRun,
				fx.Supply(eventArgs),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewSecurityAgentParams(globalParams.ConfigFilePaths),
					LogParams:    log.LogForOneShot(command.LoggerName, "info", true),
				}),
				core.Bundle,
			)
		},
		Hidden: true,
	}

	eventCmd.Flags().StringVarP(&eventArgs.sourceType, flags.SourceType, "", "compliance", "Log source name")
	eventCmd.Flags().StringVarP(&eventArgs.sourceName, flags.SourceName, "", "compliance-agent", "Log source name")
	eventCmd.Flags().StringVarP(&eventArgs.event.RuleID, flags.RuleID, "", "", "Rule ID")
	eventCmd.Flags().StringVarP(&eventArgs.event.ResourceID, flags.ResourceID, "", "", "Resource ID")
	eventCmd.Flags().StringVarP(&eventArgs.event.ResourceType, flags.ResourceType, "", "", "Resource type")
	eventCmd.Flags().StringSliceVarP(&eventArgs.event.Tags, flags.Tags, "t", []string{"security:compliance"}, "Tags")
	eventCmd.Flags().StringSliceVarP(&eventArgs.data, flags.Data, "d", []string{}, "Data KV fields")

	return eventCmd
}

func eventRun(log log.Component, config config.Component, eventArgs *cliParams) error {
	stopper := startstop.NewSerialStopper()
	defer stopper.Stop()

	endpoints, dstContext, err := command.NewLogContextCompliance(log)
	if err != nil {
		return err
	}

	runPath := config.GetString("compliance_config.run_path")
	reporter, err := compliance.NewLogReporter(stopper, eventArgs.sourceName, eventArgs.sourceType, runPath, endpoints, dstContext)
	if err != nil {
		return fmt.Errorf("failed to set up compliance log reporter: %w", err)
	}

	eventData := make(map[string]interface{})
	for _, d := range eventArgs.data {
		kv := strings.SplitN(d, ":", 2)
		if len(kv) != 2 {
			continue
		}
		eventData[kv[0]] = kv[1]
	}
	eventArgs.event.Data = eventData

	buf, err := json.Marshal(eventData)
	if err != nil {
		return err
	}
	reporter.ReportRaw(buf, "")
	return nil
}
