// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018-present Datadog, Inc.

// Package dogstatsdstats implements 'agent dogstatsd-stats'.
package dogstatsdstats

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/serverDebug"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/input"

	"github.com/spf13/cobra"
)

// cliParams are the command-line arguments for this subcommand
type cliParams struct {
	*command.GlobalParams

	// subcommand-specific flags

	dsdStatsFilePath string
	jsonStatus       bool
	prettyPrintJSON  bool
}

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &cliParams{
		GlobalParams: globalParams,
	}

	dogstatsdStatsCmd := &cobra.Command{
		Use:   "dogstatsd-stats",
		Short: "Print basic statistics on the metrics processed by dogstatsd",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(requestDogstatsdStats,
				fx.Supply(cliParams),
				fx.Supply(command.GetDefaultCoreBundleParams(cliParams.GlobalParams)),
				core.Bundle,
			)
		},
	}

	dogstatsdStatsCmd.Flags().BoolVarP(&cliParams.jsonStatus, "json", "j", false, "print out raw json")
	dogstatsdStatsCmd.Flags().BoolVarP(&cliParams.prettyPrintJSON, "pretty-json", "p", false, "pretty print JSON")
	dogstatsdStatsCmd.Flags().StringVarP(&cliParams.dsdStatsFilePath, "file", "o", "", "Output the dogstatsd-stats command to a file")

	return []*cobra.Command{dogstatsdStatsCmd}
}

func requestDogstatsdStats(log log.Component, config config.Component, cliParams *cliParams) error {
	fmt.Printf("Getting the dogstatsd stats from the agent.\n\n")
	var e error
	var s string
	c := util.GetClient(false) // FIX: get certificates right then make this true
	ipcAddress, err := pkgconfig.GetIPCAddress()
	if err != nil {
		return err
	}
	urlstr := fmt.Sprintf("https://%v:%v/agent/dogstatsd-stats", ipcAddress, pkgconfig.Datadog.GetInt("cmd_port"))

	// Set session token
	e = util.SetAuthToken()
	if e != nil {
		return e
	}

	r, e := util.DoGet(c, urlstr, util.LeaveConnectionOpen)
	if e != nil {
		var errMap = make(map[string]string)
		json.Unmarshal(r, &errMap) //nolint:errcheck
		// If the error has been marshalled into a json object, check it and return it properly
		if err, found := errMap["error"]; found {
			e = fmt.Errorf(err)
		}

		if len(errMap["error_type"]) > 0 {
			fmt.Println(e)
			return nil
		}

		fmt.Printf("Could not reach agent: %v \nMake sure the agent is running before requesting the dogstatsd stats and contact support if you continue having issues. \n", e)

		return e
	}

	// The rendering is done in the client so that the agent has less work to do
	if cliParams.prettyPrintJSON {
		var prettyJSON bytes.Buffer
		json.Indent(&prettyJSON, r, "", "  ") //nolint:errcheck
		s = prettyJSON.String()
	} else if cliParams.jsonStatus {
		s = string(r)
	} else {
		s, e = serverDebug.FormatDebugStats(r)
		if e != nil {
			fmt.Printf("Could not format the statistics, the data must be inconsistent. You may want to try the JSON output. Contact the support if you continue having issues.\n")
			return nil
		}
	}

	if cliParams.dsdStatsFilePath == "" {
		fmt.Println(s)
		return nil
	}

	// if the file is already existing, ask for a confirmation.
	if _, err := os.Stat(cliParams.dsdStatsFilePath); err == nil {
		if !input.AskForConfirmation(fmt.Sprintf("'%s' already exists, do you want to overwrite it? [y/N]", cliParams.dsdStatsFilePath)) {
			fmt.Println("Canceling.")
			return nil
		}
	}

	if err := os.WriteFile(cliParams.dsdStatsFilePath, []byte(s), 0644); err != nil {
		fmt.Println("Error while writing the file (is the location writable by the dd-agent user?):", err)
	} else {
		fmt.Println("Dogstatsd stats written in:", cliParams.dsdStatsFilePath)
	}

	return nil
}
