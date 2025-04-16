// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	"github.com/DataDog/datadog-agent/test/fakeintake/client"
)

type alertType struct {
	event.AlertType
}

// NewFilterEventsCommand returns the filter events command
func NewFilterEventsCommand(cl **client.Client) (cmd *cobra.Command) {
	var name string
	var tags []string
	var alertType alertType

	cmd = &cobra.Command{
		Use:     "events",
		Short:   "Filter events",
		Example: `fakeintakectl --url http://internal-lenaic-eks-fakeintake-2062862526.us-east-1.elb.amazonaws.com filter events --source docker`,
		RunE: func(cmd *cobra.Command, _ []string) (err error) {
			var opts []client.MatchOpt[*aggregator.Event]
			if cmd.Flag("tags").Changed {
				opts = append(opts, client.WithTags[*aggregator.Event](tags))
			}
			if cmd.Flag("alert-type").Changed {
				opts = append(opts, client.WithAlertType(alertType.AlertType))
			}

			events, err := (*cl).FilterEvents(name, opts...)
			if err != nil {
				return err
			}

			output, err := json.MarshalIndent(events, "", "  ")
			if err != nil {
				return err
			}

			fmt.Println(string(output))

			return nil
		},
	}

	cmd.Flags().StringVar(&name, "source", "", "Source of the event")
	cmd.Flags().StringSliceVar(&tags, "tags", []string{}, "Tags to filter on")
	cmd.Flags().Var(&alertType, "alert-type", "Alert type to filter on")

	if err := cmd.MarkFlagRequired("source"); err != nil {
		panic(err)
	}

	return cmd
}

func (a *alertType) Set(value string) (err error) {
	a.AlertType, err = event.GetAlertTypeFromString(value)
	return
}

func (a *alertType) String() string {
	return string(a.AlertType)
}

func (a *alertType) Type() string {
	return "alerttype"
}
