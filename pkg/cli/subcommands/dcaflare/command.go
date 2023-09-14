// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package dcaflare

import (
	"bytes"
	"fmt"

	"github.com/DataDog/datadog-agent/cmd/agent/common/path"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/flare"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/input"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"go.uber.org/fx"
)

type cliParams struct {
	caseID string
	email  string
	send   bool
}

type GlobalParams struct {
	ConfFilePath string
}

const (
	LoggerName      = "CLUSTER"
	DefaultLogLevel = "off"
)

// MakeCommand returns a `flare` command to be used by cluster-agent binaries.
func MakeCommand(globalParamsGetter func() GlobalParams) *cobra.Command {
	cliParams := &cliParams{}

	cmd := &cobra.Command{
		Use:   "flare [caseID]",
		Short: "Collect a flare and send it to Datadog",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				cliParams.caseID = args[0]
			}

			if cliParams.email == "" {
				var err error
				cliParams.email, err = input.AskForEmail()
				if err != nil {
					fmt.Println("Error reading email, please retry or contact support")
					return err
				}
			}
			globalParams := globalParamsGetter()

			return fxutil.OneShot(run,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewClusterAgentParams(globalParams.ConfFilePath, config.WithConfigLoadSecrets(true)),
					LogParams:    log.LogForOneShot(LoggerName, DefaultLogLevel, true),
				}),
				core.Bundle,
			)
		},
	}

	cmd.Flags().StringVarP(&cliParams.email, "email", "e", "", "Your email")
	cmd.Flags().BoolVarP(&cliParams.send, "send", "s", false, "Automatically send flare (don't prompt for confirmation)")
	cmd.SetArgs([]string{"caseID"})

	return cmd
}

func run(log log.Component, config config.Component, cliParams *cliParams) error {
	fmt.Fprintln(color.Output, color.BlueString("Asking the Cluster Agent to build the flare archive."))
	var e error
	c := util.GetClient(false) // FIX: get certificates right then make this true
	urlstr := fmt.Sprintf("https://localhost:%v/flare", pkgconfig.Datadog.GetInt("cluster_agent.cmd_port"))

	logFile := pkgconfig.Datadog.GetString("log_file")
	if logFile == "" {
		logFile = path.DefaultDCALogFile
	}

	// Set session token
	e = util.SetAuthToken()
	if e != nil {
		return e
	}

	r, e := util.DoPost(c, urlstr, "application/json", bytes.NewBuffer([]byte{}))
	var filePath string
	if e != nil {
		if r != nil && string(r) != "" {
			fmt.Fprintf(color.Output, "The agent ran into an error while making the flare: %s\n", color.RedString(string(r)))
		} else {
			fmt.Fprintln(color.Output, color.RedString("The agent was unable to make a full flare: %s.", e.Error()))
		}
		fmt.Fprintln(color.Output, color.YellowString("Initiating flare locally, some logs will be missing."))
		filePath, e = flare.CreateDCAArchive(true, path.GetDistPath(), logFile, aggregator.GetSenderManager())
		if e != nil {
			fmt.Printf("The flare zipfile failed to be created: %s\n", e)
			return e
		}
	} else {
		filePath = string(r)
	}

	fmt.Fprintf(color.Output, "%s is going to be uploaded to Datadog\n", color.YellowString(filePath))
	if !cliParams.send {
		confirmation := input.AskForConfirmation("Are you sure you want to upload a flare? [y/N]")
		if !confirmation {
			fmt.Fprintf(color.Output, "Aborting. (You can still use %s)\n", color.YellowString(filePath))
			return nil
		}
	}

	response, e := flare.SendFlare(filePath, cliParams.caseID, cliParams.email, "local")
	fmt.Println(response)
	if e != nil {
		return e
	}
	return nil
}
