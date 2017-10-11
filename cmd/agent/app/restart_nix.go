// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.
// +build !windows

package app

import (
	"syscall"

	log "github.com/cihub/seelog"
	"github.com/spf13/cobra"
)

var (
	restartCmd = &cobra.Command{
		Use:   "restart",
		Short: "Restart the Agent",
		Long:  `Kills parent process before starting agent in the foreground`,
		RunE:  restart,
	}
)

func init() {
	// attach the command to the root
	AgentCmd.AddCommand(restartCmd)
}

// Kill the parent process, then restart the main loop
func restart(cmd *cobra.Command, args []string) error {
	parent := syscall.Getppid()
	log.Infof("Process restarted. Killing parent - PID: %v", parent)
	syscall.Kill(parent, syscall.SIGTERM)

	return start(cmd, args)

}
