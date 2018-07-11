// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package app

import (
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/pkg/api"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	taggerapi "github.com/DataDog/datadog-agent/pkg/tagger/api"
)

func init() {
	AgentCmd.AddCommand(taggerListCommand)
}

var taggerListCommand = &cobra.Command{
	Use:   "tagger-list",
	Short: "Print the tagger content of a running agent",
	Long:  ``,
	RunE: func(cmd *cobra.Command, args []string) error {
		tr := &tagger.ListResponse{}
		err := api.RetrieveJSON("/agent/tagger-list", tr)
		if err != nil {
			return err
		}
		return taggerapi.PrintList(tr)
	},
}
