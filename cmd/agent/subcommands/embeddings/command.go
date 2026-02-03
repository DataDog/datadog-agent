// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package embeddings implements 'agent embeddings' subcommands.
package embeddings

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/pkg/deepinference"
)

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(_ *command.GlobalParams) []*cobra.Command {
	embeddingCommand := &cobra.Command{
		Use:   "embedding [text]",
		Short: "Print the embedding vector of the provided text",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if err := deepinference.Init(); err != nil {
				return err
			}

			embeddings, err := deepinference.GetEmbeddings(args[0])
			if err != nil {
				return err
			}

			fmt.Printf("Embedding vector (%d dimensions):\n", len(embeddings))
			fmt.Printf("[")
			for i, val := range embeddings {
				if i > 0 {
					fmt.Printf(", ")
				}
				if i >= 3 {
					fmt.Printf("...")
					break
				}
				fmt.Printf("%.3f", val)
			}
			fmt.Printf("]\n")

			mean := float32(0.0)
			for _, val := range embeddings {
				mean += val
			}
			mean /= float32(len(embeddings))
			fmt.Printf("Mean: %.6f\n", mean)

			return nil
		},
	}

	similarityCommand := &cobra.Command{
		Use:   "similarity [text1] [text2] ...",
		Short: "Print cosine similarities between multiple texts",
		Long:  "Computes and displays the cosine similarity matrix between all provided texts. Requires at least 2 texts.",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			if err := deepinference.Init(); err != nil {
				return err
			}

			// Get embeddings for all texts
			embeddings := make([][]float32, len(args))
			for i, text := range args {
				emb, err := deepinference.GetEmbeddings(text)
				if err != nil {
					return fmt.Errorf("failed to get embeddings for text %d (%q): %w", i+1, text, err)
				}
				embeddings[i] = emb
			}

			// Calculate and print similarity matrix
			fmt.Printf("Cosine similarity matrix:\n\n")
			fmt.Printf("%-20s", "")
			for i := range args {
				fmt.Printf("  Text %-2d", i+1)
			}
			fmt.Printf("\n")

			for i := range args {
				fmt.Printf("%-20s", fmt.Sprintf("Text %d", i+1))
				for j := range args {
					if i <= j {
						break
					}
					similarity := cosineSimilarity(embeddings[i], embeddings[j])
					fmt.Printf("  %7.4f", similarity)
				}
				fmt.Printf("\n")
			}

			fmt.Printf("\nTexts:\n")
			for i, text := range args {
				fmt.Printf("  Text %d: %q\n", i+1, text)
			}

			return nil
		},
	}

	benchmarkCommand := &cobra.Command{
		Use:   "benchmark",
		Short: "Benchmark the deepinference library",
		RunE: func(_ *cobra.Command, _ []string) error {
			if err := deepinference.Init(); err != nil {
				return err
			}

			if err := deepinference.Benchmark(); err != nil {
				return err
			}

			return nil
		},
	}

	return []*cobra.Command{embeddingCommand, similarityCommand, benchmarkCommand}
}

// cosineSimilarity calculates the cosine similarity between two embedding vectors.
// Since embeddings are L2-normalized, cosine similarity is just the dot product.
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0.0
	}

	var dotProduct float64
	for i := range a {
		dotProduct += float64(a[i] * b[i])
	}

	// Clamp to [-1, 1] to handle floating point errors
	if dotProduct > 1.0 {
		dotProduct = 1.0
	} else if dotProduct < -1.0 {
		dotProduct = -1.0
	}

	return dotProduct
}
