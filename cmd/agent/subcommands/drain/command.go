// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package drain implements 'agent drain'.
package drain

import (
	"bytes"
	"fmt"
	"os"
	"slices"

	"go.uber.org/fx"

	"github.com/faceair/drain"
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	secretsnoopfx "github.com/DataDog/datadog-agent/comp/core/secrets/fx-noop"
	"github.com/DataDog/datadog-agent/pkg/logs/processor"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// CliParams holds the command-line arguments for the drain subcommand.
type CliParams struct {
	*command.GlobalParams

	// InputFilePath represents the path to the input log file.
	InputFilePath string

	// Threshold represents the cluster size threshold for filtering logs.
	Threshold int

	// ScoreThreshold represents the score threshold for filtering logs (if set, overrides Threshold).
	ScoreThreshold *float64

	// ProgressiveTraining indicates whether to train the drain processor on all logs before filtering.
	ProgressiveTraining bool

	// LogClusterDepth represents the depth of the log cluster tree.
	LogClusterDepth int

	// SimTh represents the similarity threshold for clustering.
	SimTh float64

	// MaxChildren represents the maximum number of children in the cluster tree.
	MaxChildren int
}

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &CliParams{
		GlobalParams: globalParams,
	}

	var scoreThreshold float64
	cmd := &cobra.Command{
		Use:   "drain [filepath]",
		Short: "Filter logs using drain processor",
		Long:  `Read logs from a file, apply drain filtering, and write filtered results to stdout`,
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cliParams.InputFilePath = args[0]
			// Set ScoreThreshold pointer if flag was provided
			if scoreThreshold > 0 {
				cliParams.ScoreThreshold = &scoreThreshold
			}
			bundleParams := command.GetDefaultCoreBundleParams(cliParams.GlobalParams)
			// Enable logging at info level
			bundleParams.LogParams = log.ForOneShot(command.LoggerName, "info", true)
			return fxutil.OneShot(runDrain,
				fx.Supply(cliParams),
				fx.Supply(bundleParams),
				core.Bundle(),
				secretsnoopfx.Module(),
			)
		},
	}
	cmd.Flags().IntVarP(&cliParams.Threshold, "threshold", "t", 10, "Cluster size threshold for filtering logs (default: 10)")
	cmd.Flags().Float64Var(&scoreThreshold, "score-threshold", 0, "Score threshold for filtering logs (if set, overrides threshold)")
	cmd.Flags().BoolVarP(&cliParams.ProgressiveTraining, "progressive-training", "", true, "Train the drain processor progressively on logs before filtering")
	cmd.Flags().IntVar(&cliParams.LogClusterDepth, "log-cluster-depth", 4, "Depth of the log cluster tree (default: 4)")
	cmd.Flags().Float64Var(&cliParams.SimTh, "sim-th", 0.4, "Similarity threshold for clustering (default: 0.4)")
	cmd.Flags().IntVar(&cliParams.MaxChildren, "max-children", 100, "Maximum number of children in the cluster tree (default: 100)")

	return []*cobra.Command{cmd}
}

// runDrain reads the input file, applies drain filtering, and writes filtered logs to stdout.
func runDrain(lc log.Component, _ config.Component, cliParams *CliParams) error {
	// Create drain processor
	drainProcessor := processor.NewDrainProcessor("drain-command", &drain.Config{
		LogClusterDepth: cliParams.LogClusterDepth,
		SimTh:           cliParams.SimTh,
		MaxChildren:     cliParams.MaxChildren,
		ParamString:     "<*>",
	})

	// Read entire file and split by line feeds
	fileContent, err := os.ReadFile(cliParams.InputFilePath)
	if err != nil {
		return fmt.Errorf("error reading input file %s: %w", cliParams.InputFilePath, err)
	}

	// Split by line feeds
	lines := bytes.Split(fileContent, []byte("\n"))
	processedCount := 0
	filteredCount := 0

	if !cliParams.ProgressiveTraining {
		// Train first
		for _, line := range lines {
			tokens := processor.DrainTokenize(line)
			drainProcessor.Train(tokens)
		}
	}

	// If trained first, we can compute accurate stats about cluster distribution
	clusters := drainProcessor.Clusters()
	slices.SortFunc(clusters, func(a, b *drain.LogCluster) int {
		return a.Size() - b.Size()
	})
	totalSize := 0.0
	for _, cluster := range clusters {
		totalSize += float64(cluster.Size())
	}

	// Inference
	for _, line := range lines {
		tokens := processor.DrainTokenize(line)
		cluster := drainProcessor.Match(tokens)
		if cliParams.ProgressiveTraining {
			drainProcessor.Train(tokens)
		}
		s := 0
		if cluster != nil {
			s = cluster.Size()
		}

		// The score is ~how many logs are less similar than this log
		score := 0.0
		for _, cluster := range clusters {
			if s >= cluster.Size() {
				score += float64(cluster.Size())
			}
		}
		if totalSize > 0 {
			score /= totalSize
		}

		// Filter by score threshold if set, otherwise use size threshold
		var toIgnore bool
		if cliParams.ScoreThreshold != nil && *cliParams.ScoreThreshold > 0 {
			toIgnore = score >= *cliParams.ScoreThreshold
		} else {
			toIgnore = s >= cliParams.Threshold
		}

		// Write non-filtered lines to stdout
		if toIgnore {
			filteredCount++
		} else {
			processedCount++
			fmt.Println(string(line))
		}
	}

	fmt.Println("--------------------------------")
	slices.SortFunc(clusters, func(a, b *drain.LogCluster) int {
		return b.Size() - a.Size()
	})
	fmt.Println("Top 10 clusters:")
	for i, cluster := range clusters[:10] {
		fmt.Printf("Cluster %d: %s\n", i+1, cluster.String())
	}

	fmt.Printf("Processed %d lines: filtered %f%%\n", len(lines), float64(filteredCount)/float64(len(lines))*100)

	return nil
}
