// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package streamlogs implements 'agent stream-logs'.
package streamlogs

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	"github.com/spf13/cobra"
)

// cliParams are the command-line arguments for this subcommand
type cliParams struct {
	*command.GlobalParams

	filters diagnostic.Filters
}

type StreamLogParams struct {
	// Output represents the output file path to write the log stream to.
	FilePath string

	// Duration represents the duration for which log stream will run
	Duration time.Duration
}

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &cliParams{
		GlobalParams: globalParams,
	}

	streamLogParams := &StreamLogParams{}

	cmd := &cobra.Command{
		Use:   "stream-logs",
		Short: "Stream the logs being processed by a running agent",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(streamLogs,
				fx.Supply(cliParams),
				fx.Supply(streamLogParams),
				fx.Supply(command.GetDefaultCoreBundleParams(cliParams.GlobalParams)),
				core.Bundle(),
			)
		},
	}
	cmd.Flags().StringVar(&cliParams.filters.Name, "name", "", "Filter by name")
	cmd.Flags().StringVar(&cliParams.filters.Type, "type", "", "Filter by type")
	cmd.Flags().StringVar(&cliParams.filters.Source, "source", "", "Filter by source")
	cmd.Flags().StringVar(&cliParams.filters.Service, "service", "", "Filter by service")
	cmd.Flags().StringVarP(&streamLogParams.FilePath, "output", "o", "", "Output file path to write the log stream")
	cmd.Flags().DurationP("duration", "d", streamLogParams.Duration, "Duration to stream logs for (default 10s)")

	// PreRunE is used to validate the file path before stream-logs is run.
	cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		if streamLogParams.FilePath != "" {
			// Check if the file path's directory exists or create it.
			dir := filepath.Dir(streamLogParams.FilePath)
			if _, err := os.Stat(dir); os.IsNotExist(err) {
				// Directory does not exist, attempt to create it.
				if err := os.MkdirAll(dir, 0755); err != nil {
					return fmt.Errorf("unable to create directory path: %s, error: %v", dir, err)
				}
			} else if err != nil {
				// Some other error occurred when checking the directory.
				return fmt.Errorf("error checking directory path: %s, error: %v", dir, err)
			}
		}
		return nil
	}

	return []*cobra.Command{cmd}
}

//nolint:revive // TODO(AML) Fix revive linter
func streamLogs(log log.Component, config config.Component, cliParams *cliParams, streamLogParams *StreamLogParams) error {
	ipcAddress, err := pkgconfig.GetIPCAddress()
	if err != nil {
		return err
	}

	body, err := json.Marshal(&cliParams.filters)

	if err != nil {
		return err
	}

	urlstr := fmt.Sprintf("https://%v:%v/agent/stream-logs", ipcAddress, config.GetInt("cmd_port"))
	return streamRequest(urlstr, body, func(chunk []byte) {
		fmt.Print(string(chunk))

		if streamLogParams.FilePath != "" {
			err := writeToFile(streamLogParams.FilePath, string(chunk))
			if err != nil {
				fmt.Printf("Error writing stream-logs to file %s: %v", streamLogParams.FilePath, err)
			}
		}

	})
}

func streamRequest(url string, body []byte, onChunk func([]byte)) error {
	var e error
	c := util.GetClient(false)

	// Set session token
	e = util.SetAuthToken(pkgconfig.Datadog)
	if e != nil {
		return e
	}

	e = util.DoPostChunked(c, url, "application/json", bytes.NewBuffer(body), onChunk)

	if e == io.EOF {
		return nil
	}
	if e != nil {
		fmt.Printf("Could not reach agent: %v \nMake sure the agent is running before requesting the logs and contact support if you continue having issues. \n", e)
	}
	return e
}

func writeToFile(filePath, message string) error {
	// Open the file for writing, create it if it does not exist.
	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	// Use a buffered writer to minimize direct writes to disk.
	bufWriter := bufio.NewWriter(f)

	_, err = bufWriter.WriteString(message + "\n")
	if err != nil {
		return err
	}

	// Flush before closing the file
	if err = bufWriter.Flush(); err != nil {
		return err
	}

	return nil
}
