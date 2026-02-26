// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package validatepodannotation implements 'agent validate-pod-annotation'.
// It validates the JSON used in Kubernetes pod check annotations
// (ad.datadoghq.com/<container>.checks) and exits with an error on invalid JSON.
package validatepodannotation

import (
	"bufio"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/utils"
)

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(_ *command.GlobalParams) []*cobra.Command {
	validateCmd := &cobra.Command{
		Use:   "validate-pod-annotation [file]",
		Short: "Validate Kubernetes pod check annotation JSON",
		Long: `Validates the JSON used in Kubernetes pod check annotations
(ad.datadoghq.com/<container>.checks). Read from file or stdin.
Exits with code 1 and prints the error on invalid JSON.
Use this to catch syntax errors before applying annotations to pods.`,
		RunE: run,
	}
	validateCmd.Args = cobra.MaximumNArgs(1)
	return []*cobra.Command{validateCmd}
}

func run(_ *cobra.Command, args []string) error {
	var input []byte
	var err error
	if len(args) == 1 {
		input, err = os.ReadFile(args[0])
		if err != nil {
			return fmt.Errorf("reading file: %w", err)
		}
	} else {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			input = append(input, scanner.Bytes()...)
			input = append(input, '\n')
		}
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("reading stdin: %w", err)
		}
	}

	if err := utils.ValidatePodChecksAnnotation(string(input)); err != nil {
		return fmt.Errorf("invalid pod check annotation JSON: %w", err)
	}
	return nil
}
