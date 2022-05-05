// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package app

import (
	"fmt"
	"io"
	"runtime"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// VersionCmd is a command that prints the process-agent version data
var VersionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version info",
	RunE: func(cmd *cobra.Command, args []string) error {
		return WriteVersion(color.Output)
	},
	SilenceUsage: true,
}

// versionString returns the version information filled in at build time
func versionString(v version.Version) string {
	var meta string
	if v.Meta != "" {
		meta = fmt.Sprintf("- Meta: %s ", color.YellowString(v.Meta))
	}
	return fmt.Sprintf(
		"Agent %s %s- Commit: %s - Serialization version: %s - Go version: %s",
		color.CyanString(v.GetNumberAndPre()),
		meta,
		color.GreenString(v.Commit),
		color.YellowString(serializer.AgentPayloadVersion),
		color.RedString(runtime.Version()),
	)
}

// WriteVersion writes the version string to out
func WriteVersion(w io.Writer) error {
	v, err := version.Agent()
	if err != nil {
		return err
	}

	_, err = fmt.Fprintln(w, versionString(v))
	return err
}
