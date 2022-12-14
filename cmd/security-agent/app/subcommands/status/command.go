// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package status

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/security-agent/app/common"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/status"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type statusCliParams struct {
	*common.GlobalParams

	json            bool
	prettyPrintJSON bool
	file            string
}

func Commands(globalParams *common.GlobalParams) []*cobra.Command {
	cliParams := &statusCliParams{
		GlobalParams: globalParams,
	}

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Print the current status",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(runStatus,
				fx.Supply(cliParams),
				fx.Supply(core.CreateBundleParams(
					"",
					core.WithSecurityAgentConfigFilePaths(globalParams.ConfPathArray),
					core.WithConfigLoadSecurityAgent(true),
				).LogForOneShot(common.LoggerName, "off", true)),
				core.Bundle,
			)
		},
	}

	statusCmd.Flags().BoolVarP(&cliParams.json, "json", "j", false, "print out raw json")
	statusCmd.Flags().BoolVarP(&cliParams.prettyPrintJSON, "pretty-json", "p", false, "pretty print JSON")
	statusCmd.Flags().StringVarP(&cliParams.file, "file", "o", "", "Output the status command to a file")

	return []*cobra.Command{statusCmd}
}

func runStatus(log log.Component, config config.Component, params *statusCliParams) error {
	fmt.Printf("Getting the status from the agent.\n")
	var e error
	var s string
	c := util.GetClient(false) // FIX: get certificates right then make this true
	urlstr := fmt.Sprintf("https://localhost:%v/agent/status", config.GetInt("security_agent.cmd_port"))

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

		fmt.Printf(`
		Could not reach security agent: %v
		Make sure the agent is running before requesting the status.
		Contact support if you continue having issues.`, e)
		return e
	}

	// The rendering is done in the client so that the agent has less work to do
	if params.prettyPrintJSON {
		var prettyJSON bytes.Buffer
		json.Indent(&prettyJSON, r, "", "  ") //nolint:errcheck
		s = prettyJSON.String()
	} else if params.json {
		s = string(r)
	} else {
		formattedStatus, err := status.FormatSecurityAgentStatus(r)
		if err != nil {
			return err
		}
		s = formattedStatus
	}

	if params.file != "" {
		os.WriteFile(params.file, []byte(s), 0644) //nolint:errcheck
	} else {
		fmt.Println(s)
	}

	return nil
}
