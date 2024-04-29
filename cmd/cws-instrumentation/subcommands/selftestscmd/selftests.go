// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package selftestscmd holds the selftests command of CWS injector
package selftestscmd

import (
	"errors"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

type execParams struct {
	enabled bool
	path    string
	args    string
}

type openParams struct {
	enabled bool
	path    string
}

type selftestsCliParams struct {
	exec execParams
	open openParams
}

// Command returns the commands for the selftests subcommand
func Command() []*cobra.Command {
	var params selftestsCliParams

	selftestsCmd := &cobra.Command{
		Use:   "selftests",
		Short: "run selftests against the tracer",
		RunE: func(cmd *cobra.Command, args []string) error {
			var err error
			if params.exec.enabled {
				err = errors.Join(err, selftestExec(&params.exec))
			}
			if params.open.enabled {
				err = errors.Join(err, selftestOpen(&params.open))
			}
			return err
		},
	}

	selftestsCmd.Flags().BoolVar(&params.exec.enabled, "exec", false, "run the exec selftest")
	selftestsCmd.Flags().StringVar(&params.exec.path, "exec.path", "/usr/bin/date", "path to the file to execute")
	selftestsCmd.Flags().StringVar(&params.exec.args, "exec.args", "", "arguments to pass to the executable")
	selftestsCmd.Flags().BoolVar(&params.open.enabled, "open", false, "run the open selftest")
	selftestsCmd.Flags().StringVar(&params.open.path, "open.path", "/tmp/open.test", "path to the file to open")

	return []*cobra.Command{selftestsCmd}
}

func selftestExec(params *execParams) error {
	if params.args != "" {
		return exec.Command(params.path, strings.Split(params.args, " ")...).Run()
	}
	return exec.Command(params.path).Run()
}

func selftestOpen(params *openParams) error {
	f, createErr := os.OpenFile(params.path, os.O_CREATE|os.O_EXCL, 0400)
	if createErr != nil {
		f, openErr := os.Open(params.path)
		if openErr != nil {
			return errors.Join(createErr, openErr)
		}
		return f.Close()
	}
	return errors.Join(f.Close(), os.Remove(params.path))
}
