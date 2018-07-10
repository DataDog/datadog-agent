// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package app

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/cmd/agent/api/response"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config"

	"github.com/fatih/color"
	json "github.com/json-iterator/go"
	"github.com/spf13/cobra"
)

func init() {
	AgentCmd.AddCommand(taggerListCommand)
}

var taggerListURL = fmt.Sprintf("https://localhost:%v/agent/tagger-list", config.Datadog.GetInt("cmd_port"))

var taggerListCommand = &cobra.Command{
	Use:   "tagger-list",
	Short: "Print the tagger content of a running agent",
	Long:  ``,
	RunE: func(cmd *cobra.Command, args []string) error {
		err := common.SetupConfig(confFilePath)
		if err != nil {
			return fmt.Errorf("unable to set up global agent configuration: %v", err)
		}
		if flagNoColor {
			color.NoColor = true
		}
		c := util.GetClient(false) // FIX: get certificates right then make this true

		// Set session token
		err = util.SetAuthToken()
		if err != nil {
			return err
		}

		r, err := util.DoGet(c, taggerListURL)
		if err != nil {
			if r != nil && string(r) != "" {
				fmt.Fprintln(color.Output, fmt.Sprintf("The agent ran into an error while getting tags list: %s", string(r)))
			} else {
				fmt.Fprintln(color.Output, fmt.Sprintf("Failed to query the agent (running?): %s", err))
			}
		}

		tr := response.TaggerListResponse{}
		err = json.Unmarshal(r, &tr)
		if err != nil {
			return err
		}

		for entity, tagItem := range tr.Entities {
			fmt.Fprintln(color.Output, fmt.Sprintf("\n=== Entity %s ===", color.GreenString(entity)))

			fmt.Fprint(color.Output, "Tags: [")
			for i, tag := range tagItem.Tags {
				tagInfo := strings.Split(tag, ":")
				fmt.Fprintf(color.Output, fmt.Sprintf("%s:%s", color.BlueString(tagInfo[0]), color.CyanString(strings.Join(tagInfo[1:], ":"))))
				if i != len(tagItem.Tags)-1 {
					fmt.Fprintf(color.Output, " ")
				}
			}
			fmt.Fprintln(color.Output, "]")
			fmt.Fprint(color.Output, "Sources: [")
			for i, source := range tagItem.Sources {
				fmt.Fprintf(color.Output, fmt.Sprintf("%s", color.BlueString(source)))
				if i != len(tagItem.Sources)-1 {
					fmt.Fprintf(color.Output, " ")
				}
			}
			fmt.Fprintln(color.Output, "]")
			fmt.Fprintln(color.Output, "===")
		}

		return nil
	},
}
