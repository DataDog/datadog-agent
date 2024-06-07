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

// CliParams are the command-line arguments for this subcommand
type CliParams struct {
	*command.GlobalParams

	filters diagnostic.Filters

	// Output represents the output file path to write the log stream to.
	FilePath string

	// Duration represents the duration of the log stream.
	Duration time.Duration

	//	Quiet represents whether the log stream should be quiet.
	Quiet bool
}

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &CliParams{
		GlobalParams: globalParams,
	}

	cmd := &cobra.Command{
		Use:   "stream-logs",
		Short: "Stream the logs being processed by a running agent",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(streamLogs,
				fx.Supply(cliParams),
				fx.Supply(command.GetDefaultCoreBundleParams(cliParams.GlobalParams)),
				core.Bundle(),
			)
		},
	}
	cmd.Flags().StringVar(&cliParams.filters.Name, "name", "", "Filter by name")
	cmd.Flags().StringVar(&cliParams.filters.Type, "type", "", "Filter by type")
	cmd.Flags().StringVar(&cliParams.filters.Source, "source", "", "Filter by source")
	cmd.Flags().StringVar(&cliParams.filters.Service, "service", "", "Filter by service")
	cmd.Flags().StringVarP(&cliParams.FilePath, "output", "o", "", "Output file path to write the log stream")
	cmd.Flags().DurationVarP(&cliParams.Duration, "duration", "d", 0, "Duration of the log stream (default: 0, infinite)")
	cmd.Flags().BoolVarP(&cliParams.Quiet, "quiet", "q", false, "Quiet mode (no output to stdout)")
	// PreRunE is used to validate duration before stream-logs is run.
	cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		if cliParams.Duration < 0 {
			return fmt.Errorf("duration must be a positive value")
		}
		return nil
	}

	return []*cobra.Command{cmd}
}

//nolint:revive // TODO(AML) Fix revive linter
func streamLogs(log log.Component, config config.Component, cliParams *CliParams) error {
	ipcAddress, err := pkgconfig.GetIPCAddress()
	if err != nil {
		return err
	}

	body, err := json.Marshal(&cliParams.filters)

	if err != nil {
		return err
	}

	urlstr := fmt.Sprintf("https://%v:%v/agent/stream-logs", ipcAddress, config.GetInt("cmd_port"))

	var f *os.File
	var bufWriter *bufio.Writer

	if cliParams.FilePath != "" {
		err = checkDirExists(cliParams.FilePath)
		if err != nil {
			return fmt.Errorf("error creating directory for file %s: %v", cliParams.FilePath, err)
		}

		f, bufWriter, err = openFileForWriting(cliParams.FilePath)
		if err != nil {
			return fmt.Errorf("error opening file %s for writing: %v", cliParams.FilePath, err)
		}
		defer func() {
			err := bufWriter.Flush()
			if err != nil {
				fmt.Printf("Error flushing buffer for log stream: %v", err)
			}
			f.Close()
		}()
	}

	return streamRequest(urlstr, body, cliParams.Duration, func(chunk []byte) {
		if !cliParams.Quiet {
			fmt.Print(string(chunk))
		}

		if bufWriter != nil {
			if _, err = bufWriter.Write(chunk); err != nil {
				fmt.Printf("Error writing stream-logs to file %s: %v", cliParams.FilePath, err)
			}
		}
	})
}

func streamRequest(url string, body []byte, duration time.Duration, onChunk func([]byte)) error {
	var e error
	c := util.GetClient(false)
	if duration != 0 {
		c.Timeout = duration
	}
	// Set session token
	e = util.SetAuthToken(pkgconfig.Datadog())
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

// openFileForWriting opens a file for writing
func openFileForWriting(filePath string) (*os.File, *bufio.Writer, error) {
	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, nil, fmt.Errorf("error opening file %s: %v", filePath, err)
	}
	bufWriter := bufio.NewWriter(f) // default 4096 bytes buffer
	return f, bufWriter, nil
}

// checkDirExists checks if the directory for the given path exists, if not then create it.
func checkDirExists(path string) error {
	dir := filepath.Dir(path)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	return nil
}

// StreamLogs is a public function that can be used by other packages to stream logs.
func StreamLogs(log log.Component, config config.Component, cliParams *CliParams) error {
	return streamLogs(log, config, cliParams)
}
