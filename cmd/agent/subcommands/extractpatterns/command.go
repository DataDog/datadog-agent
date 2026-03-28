// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package extractpatterns implements 'agent extract-patterns'.
package extractpatterns

import (
	"bufio"
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/observer/impl/patterns"
)

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(_ *command.GlobalParams) []*cobra.Command {
	var summaryOnly bool

	cmd := &cobra.Command{
		Use:   "extract-patterns <file>",
		Short: "Read a file line by line and print the extracted log pattern for each line",
		Long:  `Reads a file line by line, clusters each line using the observer pattern clusterer, and prints the extracted pattern alongside the original line.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runExtractPatterns(args[0], summaryOnly)
		},
	}

	cmd.Flags().BoolVar(&summaryOnly, "summary", false, "Display only patterns sorted by count (smallest to biggest)")

	return []*cobra.Command{cmd}
}

func runExtractPatterns(path string, summaryOnly bool) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	clusterer := patterns.NewPatternClusterer(patterns.IDComputeInfo{
		Offset: 0,
		Stride: 1,
		Index:  0,
	})

	scanner := bufio.NewScanner(f)
	type lineResult struct {
		line    string
		pattern string
	}
	var lineResults []lineResult

	for scanner.Scan() {
		line := scanner.Text()
		result := clusterer.Process(line)
		if result == nil {
			continue
		}
		lineResults = append(lineResults, lineResult{line: line, pattern: result.Pattern})
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	if summaryOnly {
		clusters := clusterer.GetClusters()
		sort.Slice(clusters, func(i, j int) bool {
			return clusters[i].Count < clusters[j].Count
		})
		for _, c := range clusters {
			fmt.Printf("(%d) %s\n", c.Count, c.PatternString())
		}
		return nil
	}

	for _, lr := range lineResults {
		fmt.Printf("Line   : %s\n", lr.line)
		fmt.Printf("Pattern: %s\n", lr.pattern)
		fmt.Println()
	}
	return nil
}
