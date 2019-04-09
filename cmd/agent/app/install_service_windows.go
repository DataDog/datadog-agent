// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package app

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/StackVista/stackstate-agent/pkg/config"
	"github.com/spf13/cobra"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
)

func init() {
	AgentCmd.AddCommand(instsvcCommand)
}

var instsvcCommand = &cobra.Command{
	Use:   "installservice",
	Short: "Installs the agent within the service control manager",
	Long:  ``,
	RunE:  installService,
}

func installService(cmd *cobra.Command, args []string) error {
	exepath, err := exePath()
	if err != nil {
		return err
	}
	fmt.Printf("exepath: %s\n", exepath)

	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(config.ServiceName)
	if err == nil {
		s.Close()
		return fmt.Errorf("service %s already exists", config.ServiceName)
	}
	s, err = m.CreateService(config.ServiceName, exepath, mgr.Config{DisplayName: "Datadog Agent Service"})
	if err != nil {
		return err
	}
	defer s.Close()
	err = eventlog.InstallAsEventCreate(config.ServiceName, eventlog.Error|eventlog.Warning|eventlog.Info)
	if err != nil {
		s.Delete()
		return fmt.Errorf("SetupEventLogSource() failed: %s", err)
	}
	return nil
}

func exePath() (string, error) {
	prog := os.Args[0]
	p, err := filepath.Abs(prog)
	if err != nil {
		return "", err
	}
	fi, err := os.Stat(p)
	if err == nil {
		if !fi.Mode().IsDir() {
			return p, nil
		}
		err = fmt.Errorf("%s is directory", p)
	}
	if filepath.Ext(p) == "" {
		p += ".exe"
		fi, statErr := os.Stat(p)
		if statErr == nil {
			if !fi.Mode().IsDir() {
				return p, nil
			}
			err = fmt.Errorf("%s is directory", p)
		} else {
			err = statErr
		}
	}
	return "", err
}
