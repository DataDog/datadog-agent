// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package app

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

func init() {
	AgentCmd.AddCommand(startsvcCommand)
	AgentCmd.AddCommand(stopsvcCommand)
	AgentCmd.AddCommand(restartsvcCommand)
}

var startsvcCommand = &cobra.Command{
	Use:   "start-service",
	Short: "starts the agent within the service control manager",
	Long:  ``,
	RunE:  StartService,
}

var stopsvcCommand = &cobra.Command{
	Use:   "stopservice",
	Short: "stops the agent within the service control manager",
	Long:  ``,
	RunE:  stopService,
}

var restartsvcCommand = &cobra.Command{
	Use:   "restart-service",
	Short: "restarts the agent within the service control manager",
	Long:  ``,
	RunE:  restartService,
}

// StartService starts the agent service via the Service Control Manager
func StartService(cmd *cobra.Command, args []string) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(ServiceName)
	if err != nil {
		return fmt.Errorf("could not access service: %v", err)
	}
	defer s.Close()
	err = s.Start("is", "manual-started")
	if err != nil {
		return fmt.Errorf("could not start service: %v", err)
	}
	return nil
}

func stopService(cmd *cobra.Command, args []string) error {
	return ControlService(svc.Stop, svc.Stopped)
}

func restartService(cmd *cobra.Command, args []string) error {
	var err error
	if err = stopService(cmd, args); err == nil {
		err = StartService(cmd, args)
	}
	return err
}

// ControlService sets the service state via the Service Control Manager
func ControlService(c svc.Cmd, to svc.State) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(ServiceName)
	if err != nil {
		return fmt.Errorf("could not access service: %v", err)
	}
	defer s.Close()
	status, err := s.Control(c)
	if err != nil {
		return fmt.Errorf("could not send control=%d: %v", c, err)
	}
	timeout := time.Now().Add(10 * time.Second)
	for status.State != to {
		if timeout.Before(time.Now()) {
			return fmt.Errorf("timeout waiting for service to go to state=%d", to)
		}
		time.Sleep(300 * time.Millisecond)
		status, err = s.Query()
		if err != nil {
			return fmt.Errorf("could not retrieve service status: %v", err)
		}
	}
	return nil
}
