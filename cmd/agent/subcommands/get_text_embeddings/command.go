// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package get_text_embeddings implements 'agent get-text-embeddings'.
package get_text_embeddings

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
)

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(_ *command.GlobalParams) []*cobra.Command {
	getTextEmbeddingsCommand := &cobra.Command{
		Use:   "get-text-embeddings [text]",
		Short: "Print the provided text using the Rust implementation",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if err := Init(); err != nil {
				return err
			}

			embeddings, err := GetEmbeddings(args[0])
			if err != nil {
				return err
			}

			fmt.Printf("First embeddings: %v\n", embeddings[0])

			return nil
		},
	}

	return []*cobra.Command{getTextEmbeddingsCommand}
}
