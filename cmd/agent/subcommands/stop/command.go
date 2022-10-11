// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows
// +build !windows

// Package stop implements 'agent stop'.
package stop

import (
	"bytes"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config"
)

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	stopCmd := &cobra.Command{
		Use:   "stop",
		Short: "Stops a running Agent",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			return stop(globalParams, cmd, args)
		},
	}

	return []*cobra.Command{stopCmd}
}

func stop(globalParams *command.GlobalParams, cmd *cobra.Command, args []string) error {
	// Global Agent configuration
	err := common.SetupConfigWithoutSecrets(globalParams.ConfFilePath, "")
	if err != nil {
		return fmt.Errorf("unable to set up global agent configuration: %v", err)
	}
	c := util.GetClient(false) // FIX: get certificates right then make this true

	// Set session token
	e := util.SetAuthToken()
	if e != nil {
		return e
	}
	ipcAddress, err := config.GetIPCAddress()
	if err != nil {
		return err
	}
	urlstr := fmt.Sprintf("https://%v:%v/agent/stop", ipcAddress, config.Datadog.GetInt("cmd_port"))

	_, e = util.DoPost(c, urlstr, "application/json", bytes.NewBuffer([]byte{}))
	if e != nil {
		return fmt.Errorf("Error stopping the agent: %v", e)
	}

	fmt.Println("Agent successfully stopped")
	return nil
}
