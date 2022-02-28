// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/spf13/cobra"
)

const targetProcessName = "system-probe"

func init() {
	SysprobeCmd.AddCommand(debugCommand)
}

var (
	debugCommand = &cobra.Command{
		Use:   "debug [state]",
		Short: "Print the runtime state of a running system-probe",
		Long:  ``,
		Args:  cobra.MinimumNArgs(1),
		RunE:  debugRuntime,
	}
)

func debugRuntime(_ *cobra.Command, args []string) error {
	c, err := getSystemProbeClient()
	if err != nil {
		return err
	}

	// TODO rather than allowing arbitrary query params, use cobra flags
	r, err := util.DoGet(c, "http://localhost/debug/"+args[0], util.CloseConnection)
	if err != nil {
		var errMap = make(map[string]string)
		_ = json.Unmarshal(r, &errMap)
		// If the error has been marshalled into a json object, check it and return it properly
		if e, found := errMap["error"]; found {
			return fmt.Errorf(e)
		}

		return fmt.Errorf("Could not reach %s: %v \nMake sure the %s is running before requesting the runtime configuration and contact support if you continue having issues", targetProcessName, err, targetProcessName)
	}

	s, err := strconv.Unquote(string(r))
	if err != nil {
		fmt.Println(string(r))
		return nil
	}
	fmt.Println(s)
	return nil
}

func getSystemProbeClient() (*http.Client, error) {
	cfg, err := setupConfig()
	if err != nil {
		return nil, err
	}
	return api.GetClient(cfg.SocketAddress), nil
}
