// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Command scenariorun is the dynamic CLI over the scenario registry. Its command
// tree (flags included) is built by reflecting each scenario's tagged params.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/scenario"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/standalone"
	"github.com/spf13/cobra"
)

func main() {
	registerScenarios()
	if err := rootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	root := &cobra.Command{Use: "scenariorun", Short: "Drive e2e scenarios"}
	root.AddCommand(listCmd(), describeCmd(), createCmd(), actionCmd(), destroyCmd(), psCmd())
	return root
}

func newCtx() *standalone.Context {
	dir, _ := os.MkdirTemp("", "scenariorun-")
	return standalone.NewContext(dir)
}

func listCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List scenarios",
		RunE: func(cmd *cobra.Command, _ []string) error {
			for _, r := range scenario.List() {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", r.Name(), r.Description())
			}
			return nil
		},
	}
}

func describeCmd() *cobra.Command {
	var asJSON bool
	c := &cobra.Command{
		Use:   "describe",
		Short: "Describe scenarios (machine-readable with --json)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			d, err := scenario.Describe()
			if err != nil {
				return err
			}
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(d)
			}
			for _, sd := range d.Scenarios {
				fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", sd.Name, sd.Description)
			}
			return nil
		},
	}
	c.Flags().BoolVar(&asJSON, "json", false, "emit JSON schema")
	return c
}

// createCmd builds one subcommand per scenario, each with schema-derived flags.
func createCmd() *cobra.Command {
	c := &cobra.Command{Use: "create <scenario>", Short: "Provision a scenario"}
	for _, r := range scenario.List() {
		r := r
		sc, err := r.ParamsSchema()
		if err != nil {
			panic(err)
		}
		sub := &cobra.Command{
			Use:   r.Name(),
			Short: r.Description(),
			RunE: func(cmd *cobra.Command, _ []string) error {
				cfg := scenario.CollectFlags(sc, cmd.Flags())
				stack, _ := cmd.Flags().GetString("stack")
				return scenario.Create(newCtx(), r.Name(), stack, cfg)
			},
		}
		scenario.RegisterFlags(sc, sub.Flags())
		sub.Flags().String("stack", r.Name()+"-stack", "Pulumi stack name")
		c.AddCommand(sub)
	}
	return c
}

func actionCmd() *cobra.Command {
	c := &cobra.Command{Use: "action <scenario> <action>", Short: "Run a scenario action"}
	for _, r := range scenario.List() {
		r := r
		actions, err := r.ActionSchemas()
		if err != nil {
			panic(err)
		}
		scenarioCmd := &cobra.Command{Use: r.Name(), Short: r.Description()}
		for name, asc := range actions {
			name, asc := name, asc
			actSub := &cobra.Command{
				Use: name,
				RunE: func(cmd *cobra.Command, _ []string) error {
					cfg := scenario.CollectFlags(asc, cmd.Flags())
					stack, _ := cmd.Flags().GetString("stack")
					return scenario.RunAction(newCtx(), r.Name(), stack, name, cfg)
				},
			}
			scenario.RegisterFlags(asc, actSub.Flags())
			actSub.Flags().String("stack", r.Name()+"-stack", "Pulumi stack name")
			scenarioCmd.AddCommand(actSub)
		}
		c.AddCommand(scenarioCmd)
	}
	return c
}

func psCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ps",
		Short: "List provisioned scenario stacks",
		RunE: func(cmd *cobra.Command, _ []string) error {
			stacks, err := scenario.ListProvisionedStacks()
			if err != nil {
				return err
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "STACK\tSCENARIO\tCREATED")
			for _, ps := range stacks {
				fmt.Fprintf(w, "%s\t%s\t%s\n", ps.Stack, ps.Scenario, ps.CreatedAt.Format(time.RFC3339))
			}
			return w.Flush()
		},
	}
}

func destroyCmd() *cobra.Command {
	c := &cobra.Command{Use: "destroy <scenario>", Short: "Tear down a scenario"}
	for _, r := range scenario.List() {
		r := r
		sub := &cobra.Command{
			Use: r.Name(),
			RunE: func(cmd *cobra.Command, _ []string) error {
				stack, _ := cmd.Flags().GetString("stack")
				return scenario.Destroy(newCtx(), r.Name(), stack)
			},
		}
		sub.Flags().String("stack", r.Name()+"-stack", "Pulumi stack name")
		c.AddCommand(sub)
	}
	return c
}

