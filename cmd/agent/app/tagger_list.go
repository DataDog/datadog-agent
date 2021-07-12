// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package app

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/DataDog/datadog-agent/cmd/agent/api/response"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func init() {
	AgentCmd.AddCommand(taggerListCommand)
}

var taggerListCommand = &cobra.Command{
	Use:   "tagger-list",
	Short: "Print the tagger content of a running agent",
	Long:  ``,
	RunE: func(cmd *cobra.Command, args []string) error {

		if flagNoColor {
			color.NoColor = true
		}

		err := common.SetupConfigWithoutSecrets(confFilePath, "")
		if err != nil {
			return fmt.Errorf("unable to set up global agent configuration: %v", err)
		}

		err = config.SetupLogger(loggerName, config.GetEnvDefault("DD_LOG_LEVEL", "off"), "", "", false, true, false)
		if err != nil {
			fmt.Printf("Cannot setup logger, exiting: %v\n", err)
			return err
		}

		c := util.GetClient(false) // FIX: get certificates right then make this true

		// Set session token
		err = util.SetAuthToken()
		if err != nil {
			return err
		}
		ipcAddress, err := config.GetIPCAddress()
		if err != nil {
			return err
		}
		r, err := util.DoGet(c, fmt.Sprintf("https://%v:%v/agent/tagger-list", ipcAddress, config.Datadog.GetInt("cmd_port")))
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

			sources := make([]string, 0, len(tagItem.Tags))
			for source := range tagItem.Tags {
				sources = append(sources, source)
			}

			// sort sources for deterministic output
			sort.Slice(sources, func(i, j int) bool {
				return sources[i] < sources[j]
			})

			for _, source := range sources {
				fmt.Fprintln(color.Output, fmt.Sprintf("== Source %s ==", source))

				fmt.Fprint(color.Output, "Tags: [")

				// sort tags for easy comparison
				tags := tagItem.Tags[source]
				sort.Slice(tags, func(i, j int) bool {
					return tags[i] < tags[j]
				})

				for i, tag := range tags {
					tagInfo := strings.Split(tag, ":")
					fmt.Fprintf(color.Output, fmt.Sprintf("%s:%s", color.BlueString(tagInfo[0]), color.CyanString(strings.Join(tagInfo[1:], ":"))))
					if i != len(tags)-1 {
						fmt.Fprintf(color.Output, " ")
					}
				}

				fmt.Fprintln(color.Output, "]")
			}

			fmt.Fprintln(color.Output, "===")
		}

		return nil
	},
}
