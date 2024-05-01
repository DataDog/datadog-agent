// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package streamlogs implements 'agent stream-logs'.
package streamlogs

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

	// Output represents the output file path to write the log stream to.
	FilePath string

	// Duration represents the duration of the log stream.
	Duration time.Duration
}

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &cliParams{
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
	// PreRunE is used to validate the file path before stream-logs is run.
	cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		if cliParams.FilePath != "" {
			// Check if the file path's directory exists or create it.
			dir := filepath.Dir(cliParams.FilePath)
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
func streamLogs(log log.Component, config config.Component, cliParams *cliParams) error {
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
		f, bufWriter, err = openFileForWriting(cliParams.FilePath)
		if err != nil {
			return fmt.Errorf("error opening file %s for writing: %v", cliParams.FilePath, err)
		}
		defer f.Close()
		defer func() {
			err := bufWriter.Flush()
			if err != nil {
				fmt.Printf("Error flushing buffer for log stream: %v", err)
			}
		}()
	}

	return streamRequest(urlstr, body, cliParams.Duration, func(chunk []byte) {
		fmt.Print(string(chunk))

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

	// Set session token
	e = util.SetAuthToken(pkgconfig.Datadog)
	if e != nil {
		return e
	}

	// Set the timeout/duration context for the stream log request
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	e = DoPostChunkedWithTimeout(ctx, c, url, "application/json", bytes.NewBuffer(body), duration, onChunk)

	if e == context.DeadlineExceeded {
		fmt.Printf("Finished streaming logs for %v\n", duration)
		return nil
	}

	if e == io.EOF {
		return nil
	}
	if e != nil {
		fmt.Printf("Could not reach agent: %v \nMake sure the agent is running before requesting the logs and contact support if you continue having issues. \n", e)
	}
	return e
}

func openFileForWriting(filePath string) (*os.File, *bufio.Writer, error) {
	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, nil, fmt.Errorf("error opening file %s: %v", filePath, err)
	}
	bufWriter := bufio.NewWriter(f) // default 4096 bytes buffer
	return f, bufWriter, nil
}

func DoPostChunkedWithTimeout(ctx context.Context, c *http.Client, url string, contentType string, body io.Reader, duration time.Duration, onChunk func([]byte)) error {
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, duration)
	defer cancel()

	done := make(chan bool)
	var err error

	go func() {
		err = util.DoPostChunked(c, url, contentType, body, onChunk)
		done <- true
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return err
	}
}
