// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package drain implements 'agent drain'.
package drain

import (
	"bufio"
	"fmt"
	"os"

	"go.uber.org/fx"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	secretsnoopfx "github.com/DataDog/datadog-agent/comp/core/secrets/fx-noop"
	"github.com/DataDog/datadog-agent/pkg/logs/processor"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// CliParams holds the command-line arguments for the drain subcommand.
type CliParams struct {
	*command.GlobalParams

	// InputFilePath represents the path to the input log file.
	InputFilePath string

	// OutputFilePath represents the path to the output file (optional, defaults to stdout).
	OutputFilePath string
}

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &CliParams{
		GlobalParams: globalParams,
	}

	cmd := &cobra.Command{
		Use:   "drain [filepath]",
		Short: "Filter logs using drain processor",
		Long:  `Read logs from a file, apply drain filtering, and write filtered results to output`,
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cliParams.InputFilePath = args[0]
			return fxutil.OneShot(runDrain,
				fx.Supply(cliParams),
				fx.Supply(command.GetDefaultCoreBundleParams(cliParams.GlobalParams)),
				core.Bundle(),
				secretsnoopfx.Module(),
			)
		},
	}
	cmd.Flags().StringVarP(&cliParams.OutputFilePath, "output", "o", "", "Output file path (default: stdout)")

	return []*cobra.Command{cmd}
}

// runDrain reads the input file, applies drain filtering, and writes filtered logs to output.
func runDrain(lc log.Component, config config.Component, cliParams *CliParams) error {
	threshold := 10
	trainFirst := false

	// Open input file
	inputFile, err := os.Open(cliParams.InputFilePath)
	if err != nil {
		return fmt.Errorf("error opening input file %s: %w", cliParams.InputFilePath, err)
	}
	defer inputFile.Close()

	// Determine output destination
	var outputFile *os.File
	var outputWriter *bufio.Writer
	if cliParams.OutputFilePath != "" {
		if err = filesystem.EnsureParentDirsExist(cliParams.OutputFilePath); err != nil {
			return fmt.Errorf("error creating directory for file %s: %w", cliParams.OutputFilePath, err)
		}

		lc.Infof("Opening file %s for writing filtered logs", cliParams.OutputFilePath)
		outputFile, outputWriter, err = filesystem.OpenFileForWriting(cliParams.OutputFilePath)
		if err != nil {
			return fmt.Errorf("error opening file %s for writing: %w", cliParams.OutputFilePath, err)
		}
		defer func() {
			if outputWriter != nil {
				if err := outputWriter.Flush(); err != nil {
					lc.Errorf("Error flushing buffer: %v", err)
				}
			}
			if outputFile != nil {
				outputFile.Close()
			}
		}()
	}

	// Create drain processor
	drainProcessor := processor.NewDrainProcessor("drain-command")

	// Read and process file line by line
	scanner := bufio.NewScanner(inputFile)
	lineNumber := 0
	processedCount := 0
	filteredCount := 0

	lines := make([][]byte, 0)
	for scanner.Scan() {
		line := scanner.Bytes()
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading input file: %w", err)
	}

	// Train first
	if trainFirst {
		for _, line := range lines {
			tokens := processor.DrainTokenize(line)
			drainProcessor.Train(tokens)
		}
	}

	// Inference
	for _, line := range lines {
		tokens := processor.DrainTokenize(line)
		cluster := drainProcessor.Match(tokens)
		if !trainFirst {
			drainProcessor.Train(tokens)
		}
		toIgnore := cluster != nil && cluster.Size() >= threshold

		// Write non-filtered lines to output
		if !toIgnore {
			processedCount++
			if outputWriter != nil {
				// Write to file
				if _, err := outputWriter.Write(line); err != nil {
					return fmt.Errorf("error writing to output file: %w", err)
				}
				if _, err := outputWriter.WriteString("\n"); err != nil {
					return fmt.Errorf("error writing newline to output file: %w", err)
				}
			} else {
				// Write to stdout
				fmt.Println(string(line))
			}
		} else {
			filteredCount++
		}
	}

	// Flush output buffer if writing to file
	if outputWriter != nil {
		if err := outputWriter.Flush(); err != nil {
			return fmt.Errorf("error flushing output buffer: %w", err)
		}
	}

	drainProcessor.ShowClusters()

	fmt.Printf("Processed %d lines: %d written, %d filtered\n", lineNumber, processedCount, filteredCount)

	return nil
}
