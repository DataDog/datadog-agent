// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package dumpconfig implements 'agent dumpconfig'.
//
// It initializes the configuration defaults (without reading any datadog.yaml
// or system-probe.yaml file) and prints the resulting runtime configuration as
// JSON. It is used by CI to verify that regenerating the config settings code
// (`dda inv schema.codegen`) does not change the Agent's runtime defaults.
package dumpconfig

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

// cliParams are the command-line arguments for this subcommand
type cliParams struct {
	*command.GlobalParams
	Target string
}

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &cliParams{
		GlobalParams: globalParams,
	}

	dumpConfigCommand := &cobra.Command{
		Use:    "dumpconfig",
		Short:  "Dump the runtime configuration defaults as JSON",
		Long:   ``,
		Hidden: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			return run(cliParams)
		},
	}
	dumpConfigCommand.Flags().StringVar(&cliParams.Target, "target", "", "config to dump: core or system-probe")

	return []*cobra.Command{dumpConfigCommand}
}

func run(cliParams *cliParams) error {
	// Initialize the global config objects with their defaults only. No
	// datadog.yaml or system-probe.yaml file is loaded, so what we dump is the
	// pure set of runtime defaults.
	pkgconfigsetup.InitConfigObjects()

	var cfg model.Config
	switch cliParams.Target {
	case "core":
		cfg = pkgconfigsetup.Datadog()
	case "system-probe":
		cfg = pkgconfigsetup.SystemProbe()
	default:
		return fmt.Errorf("unknown target '%s', valid ones are 'core' or 'system-probe'", cliParams.Target)
	}

	// json.Marshal sorts map keys, so the output is deterministic across runs.
	data, err := json.MarshalIndent(durationsToString(cfg.AllSettings()), "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

// durationsToString recursively walks a value decoded from the config and
// replaces every time.Duration with its string representation (e.g. "10s"),
// so durations are dumped as human-readable strings instead of raw
// nanosecond integers.
func durationsToString(v interface{}) interface{} {
	switch val := v.(type) {
	case time.Duration:
		return val.String()
	case map[string]interface{}:
		for k, elem := range val {
			val[k] = durationsToString(elem)
		}
		return val
	case []interface{}:
		for i, elem := range val {
			val[i] = durationsToString(elem)
		}
		return val
	default:
		return v
	}
}
