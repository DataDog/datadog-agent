// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build !windows

package app

import (
	"bytes"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config"
)

var (
	stopCmd = &cobra.Command{
		Use:   "stop",
		Short: "Stops a running Agent",
		Long:  ``,
		RunE:  stop,
	}
)

func init() {
	// attach the command to the root
	AgentCmd.AddCommand(stopCmd)
}

func stop(*cobra.Command, []string) error {
	c := util.GetClient(false) // FIX: get certificates right then make this true

	// Set session token
	e := util.SetAuthToken()
	if e != nil {
		return e
	}

	urlstr := fmt.Sprintf("https://localhost:%v/agent/stop", config.Datadog.GetInt("cmd_port"))

	_, e = util.DoPost(c, urlstr, "application/json", bytes.NewBuffer([]byte{}))
	if e != nil {
		return fmt.Errorf("Error stopping the agent: %v", e)
	}

	fmt.Println("Agent successfully stopped")
	return nil
}
