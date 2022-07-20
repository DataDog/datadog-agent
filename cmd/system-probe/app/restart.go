// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package app

import (
	"fmt"
	"io"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api"
	"github.com/spf13/cobra"
)
import "github.com/DataDog/datadog-agent/pkg/traceinit"


func init() {
	traceinit.TraceFunction(`\DataDog\datadog-agent\cmd\system-probe\app\restart.go 16`)
	SysprobeCmd.AddCommand(moduleRestartCommand)
	traceinit.TraceFunction(`\DataDog\datadog-agent\cmd\system-probe\app\restart.go 17`)
}

var moduleRestartCommand = &cobra.Command{
	Use:   "module-restart [module]",
	Short: "Restart a given system-probe module",
	Long:  ``,
	RunE:  moduleRestart,
	Args:  cobra.ExactArgs(1),
}

func moduleRestart(_ *cobra.Command, args []string) error {
	cfg, err := setupConfig()
	if err != nil {
		return err
	}
	client := api.GetClient(cfg.SocketAddress)
	url := fmt.Sprintf("http://localhost/module-restart/%s", args[0])
	resp, err := client.Post(url, "", nil)
	if err != nil {
		return err
	}

	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		err := fmt.Errorf("error restarting module: %s", body)
		return err
	}

	return nil
}