// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package common implements utils for the run command
package common

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	osComp "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/components/remote"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"

	"testing"
)

type fakeContext struct {
	// RemoteHost unfortunately requires a *testing.T which we don't
	// have since we aren't running via `go test`.
	// TODO: Would it be better to run these commands through `go test` or
	//       to make RemoteHost not require a *testing.T?
	t testing.T
}

func (f *fakeContext) T() *testing.T {
	return &f.t
}

// CreateRemoteHost creates a RemoteHost from the command line flags
func CreateRemoteHost(cmd *cobra.Command) (*components.RemoteHost, error) {
	host, err := cmd.Flags().GetString("host")
	if err != nil {
		return nil, err
	}
	username, err := cmd.Flags().GetString("username")
	if err != nil {
		return nil, err
	}

	h := &components.RemoteHost{
		HostOutput: remote.HostOutput{
			Address:   host,
			Username:  username,
			OSFamily:  osComp.WindowsFamily,
			OSFlavor:  osComp.WindowsServer,
			OSVersion: "2022",
		},
	}

	err = h.Init(&fakeContext{})
	if err != nil {
		return nil, err
	}

	return h, nil
}

// GetOutputDir creates and returns a directory that can be used to store output
func GetOutputDir(cmd *cobra.Command) string {
	output, _ := cmd.Flags().GetString("output")
	output = filepath.Join(output, "cmd-run")
	_ = os.MkdirAll(output, 0755)
	return output
}

// Init initializes the common flags
func Init(rootCmd *cobra.Command) {
	rootCmd.PersistentFlags().StringP("host", "H", "", "The host to connect to")
	_ = rootCmd.MarkPersistentFlagRequired("host")
	rootCmd.PersistentFlags().StringP("username", "u", "Administrator", "The username to connect with")
	rootCmd.PersistentFlags().StringP("output", "o", os.TempDir(), "The output directory")
}
