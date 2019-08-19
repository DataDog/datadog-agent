// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package main

import (
	"os"

	"github.com/DataDog/datadog-agent/pkg/pidfile"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the agent",
	Long:  "Run runs the trace-agent in the foreground",
	RunE: func(_ *cobra.Command, _ []string) error {
		if pidFilePath != "" {
			err := pidfile.WritePID(pidFilePath)
			if err != nil {
				log.Criticalf("Error writing PID file, exiting: %v", err)
				return err
			}
			log.Infof("PID '%d' written to PID file '%s'", os.Getpid(), pidFilePath)
			defer os.Remove(pidFilePath)
		}

		runAgent()
		return nil
	},
}

// pidFilePath specifies the location where the PID of the process should be
// written to, on disk. If it is empty, no file is written.
var pidFilePath string

func init() {
	runCmd.Flags().StringVarP(&pidFilePath, "pid", "p", "", "path to PID file; disabled when not set")
	rootCmd.AddCommand(runCmd)
}
