// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package main is a standalone CLI that provisions agent-health demo
// environments by driving the e2e framework directly.
//
// It replaces the former `dda lab demo agent-health vm create` command while
// keeping the same developer experience — no dda dependency. The command reads
// ~/.test_infra_config.yaml (keypair, API key, Pulumi passphrase) through the
// e2e-framework local profile and runs the Pulumi program in-process via the
// Automation API.
//
// Build the binary once, then call it directly (AWS credentials are taken from
// the environment, so run it under aws-vault):
//
//	cd test/new-e2e
//	go build -o bin/agent-health-demo ./tests/agent-health/cmd/agent-health-demo
//	aws-vault exec sso-agent-sandbox-account-admin -- \
//	  ./bin/agent-health-demo create --scenario invalid-config
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/spf13/cobra"

	commonconfig "github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner/parameters"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/infra"
	agenthealth "github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-health"
)

func main() {
	if err := rootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "agent-health-demo",
		Short: "Provision agent-health demo environments on AWS agent-sandbox",
		Long: "Provision agent-health demo environments by driving the e2e framework directly.\n" +
			"Run under aws-vault so AWS credentials are available.",
		SilenceUsage: true,
	}
	root.AddCommand(createCmd(), runCmd(), listCmd(), deleteCmd())
	return root
}

type createOptions struct {
	stackName    string
	infraEnv     string
	site         string
	agentVersion string
	pipelineID   string
	tags         string
	fakeintake   string
	scenario     string
	apiKey       string
}

func createCmd() *cobra.Command {
	opts := &createOptions{}
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Provision an agent-health vm demo environment",
		Long: "Provision an agent-health vm demo environment.\n\n" +
			"The API key is resolved from --api-key, E2E_API_KEY, or ~/.test_infra_config.yaml.",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runCreate(opts)
		},
	}

	scenarioHelp := "Scenario to demonstrate. Available: " + strings.Join(scenarioNames(), ", ")
	f := cmd.Flags()
	f.StringVarP(&opts.stackName, "stack-name", "i", agenthealth.DefaultStackName,
		"Stack name (prefixed with your username by the framework)")
	f.StringVar(&opts.infraEnv, "infra-env", "aws/agent-sandbox",
		"Pulumi infra environment name (e.g. aws/agent-sandbox)")
	f.StringVar(&opts.site, "site", "datad0g.com",
		"Datadog site to report to (e.g. datad0g.com, datadoghq.com)")
	f.StringVar(&opts.agentVersion, "agent-version", "",
		"Explicit agent version to install, e.g. 7.57.0 (overrides --pipeline-id)")
	f.StringVar(&opts.pipelineID, "pipeline-id", "",
		"CI pipeline ID whose build artifacts to install")
	f.StringVar(&opts.tags, "tags", "",
		"Comma-separated tags applied to AWS resources and the Datadog agent")
	f.StringVar(&opts.fakeintake, "fakeintake", "false",
		"Enable fakeintake instead of reporting to real Datadog (for E2E tests)")
	f.StringVarP(&opts.scenario, "scenario", "s", "", scenarioHelp)
	f.StringVar(&opts.apiKey, "api-key", "",
		"Datadog API key (overrides E2E_API_KEY and the config file)")

	return cmd
}

func scenarioNames() []string {
	names := make([]string, 0, len(agenthealth.DemoScenarios))
	for name := range agenthealth.DemoScenarios {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func runCreate(opts *createOptions) error {
	if opts.scenario != "" {
		if _, ok := agenthealth.DemoScenarios[opts.scenario]; !ok {
			return fmt.Errorf("unknown scenario %q; available: %s", opts.scenario, strings.Join(scenarioNames(), ", "))
		}
	}

	ctx := context.Background()

	// Scenario-specific Pulumi config. Keypair, API key, passphrase and profile
	// tags are merged automatically by the framework from ~/.test_infra_config.yaml.
	cfg := runner.ConfigMap{}
	cfg.Set(runner.InfraEnvironmentVariables, opts.infraEnv, false)
	cfg.Set(agentKey(commonconfig.DDAgentSite), opts.site, false)
	cfg.Set(agentKey(commonconfig.DDAgentFakeintake), opts.fakeintake, false)
	if opts.agentVersion != "" {
		cfg.Set(agentKey(commonconfig.DDAgentVersionParamName), opts.agentVersion, false)
	}
	if opts.pipelineID != "" {
		cfg.Set(runner.AgentPipelineID, opts.pipelineID, false)
	}
	if opts.tags != "" {
		cfg.Set(agentKey(commonconfig.DDAgentTags), opts.tags, false)
		cfg.Set(runner.InfraExtraResourcesTags, opts.tags, false)
	}
	if opts.scenario != "" {
		cfg.Set("demolab:demoScenario", opts.scenario, false)
		for k, v := range agenthealth.DemoScenarios[opts.scenario].CreateDefaults {
			cfg.Set(k, v, false)
		}
	}
	if opts.apiKey != "" {
		cfg.Set(runner.AgentAPIKey, opts.apiKey, true)
	}

	stackName := opts.stackName
	if opts.scenario != "" {
		stackName = fmt.Sprintf("%s-%s", stackName, opts.scenario)
	}

	fmt.Printf("Provisioning agent-health demo environment %q ...\n", stackName)

	sm := infra.GetStackManager()
	_, upResult, err := sm.GetStack(ctx, stackName, cfg, agenthealth.AgentHealthDemoRun, false)
	if err != nil {
		return fmt.Errorf("failed to provision stack %q: %w (run with `agent-health-demo delete` to clean up)", stackName, err)
	}

	hostIP := extractHostIP(upResult)
	if err := saveRecord(envRecord{
		Name:     stackName,
		Scenario: opts.scenario,
		HostIP:   hostIP,
		SSHUser:  agenthealth.SSHUser,
		Site:     opts.site,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not save environment record: %v\n", err)
	}

	fmt.Printf("Environment %q created.\n", stackName)
	if hostIP != "" {
		fmt.Printf("  Host IP : %s\n", hostIP)
		fmt.Printf("  SSH     : ssh %s@%s\n", agenthealth.SSHUser, hostIP)
		fmt.Printf("  Datadog : https://app.%s\n", opts.site)
	} else {
		fmt.Println("  (could not read host IP from stack outputs)")
	}
	return nil
}

// sshKeyFromProfile reads the AWS private key path and passphrase from the local
// profile (~/.test_infra_config.yaml), for SSH access during `run`.
func sshKeyFromProfile() (keyPath, password string) {
	profile := runner.GetProfile()
	if v, err := profile.ParamStore().Get(parameters.AWSPrivateKeyPath); err == nil {
		keyPath = v
	}
	if v, err := profile.SecretStore().Get(parameters.AWSPrivateKeyPassword); err == nil {
		password = v
	}
	return keyPath, password
}

func runCmd() *cobra.Command {
	var (
		id       string
		scenario string
		action   string
	)
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run a scenario action (e.g. retrigger, remediate) over SSH on a demo host",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runAction(id, scenario, action)
		},
	}
	f := cmd.Flags()
	f.StringVarP(&id, "id", "i", "", "Environment id (default: most recent matching --scenario)")
	f.StringVarP(&scenario, "scenario", "s", "", "Scenario name. Available: "+strings.Join(scenarioNames(), ", "))
	f.StringVarP(&action, "action", "a", "", "Action to run (e.g. retrigger, remediate)")
	return cmd
}

func runAction(id, scenario, action string) error {
	var rec envRecord
	var err error
	if id != "" {
		rec, err = loadRecord(id)
		if err != nil {
			return fmt.Errorf("environment %q not found: %w", id, err)
		}
	} else {
		rec, err = latestRecord(scenario)
		if err != nil {
			return err
		}
	}

	scenarioName := scenario
	if scenarioName == "" {
		scenarioName = rec.Scenario
	}
	sc, ok := agenthealth.DemoScenarios[scenarioName]
	if !ok {
		return fmt.Errorf("unknown or unset scenario %q; available: %s", scenarioName, strings.Join(scenarioNames(), ", "))
	}
	if action == "" {
		return fmt.Errorf("--action/-a is required; available: %s", strings.Join(actionNames(sc), ", "))
	}
	act, ok := sc.Actions[action]
	if !ok {
		return fmt.Errorf("unknown action %q for scenario %q; available: %s", action, scenarioName, strings.Join(actionNames(sc), ", "))
	}
	if rec.HostIP == "" {
		return fmt.Errorf("environment %q has no host IP recorded", rec.Name)
	}

	keyPath, password := sshKeyFromProfile()
	fmt.Printf("Running %q for issue %q on %s ...\n", action, sc.Issue, rec.HostIP)
	if err := runSSHCommands(rec.HostIP, rec.SSHUser, act.Commands, keyPath, password); err != nil {
		return err
	}
	fmt.Println(act.Message)
	return nil
}

func actionNames(sc agenthealth.Scenario) []string {
	names := make([]string, 0, len(sc.Actions))
	for n := range sc.Actions {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func listCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List provisioned demo environments",
		RunE: func(_ *cobra.Command, _ []string) error {
			recs, err := loadAllRecords()
			if err != nil {
				return err
			}
			if len(recs) == 0 {
				fmt.Println("No demo environments.")
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tSCENARIO\tHOST IP\tCREATED")
			for _, r := range recs {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", r.Name, dash(r.Scenario), dash(r.HostIP), r.CreatedAt)
			}
			return w.Flush()
		},
	}
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func deleteCmd() *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Destroy a demo environment and remove its record",
		RunE: func(_ *cobra.Command, _ []string) error {
			if id == "" {
				return errors.New("--id/-i is required")
			}
			return deleteEnv(id)
		},
	}
	cmd.Flags().StringVarP(&id, "id", "i", "", "Environment id to delete")
	return cmd
}

func deleteEnv(id string) error {
	ctx := context.Background()
	fmt.Printf("Destroying demo environment %q ...\n", id)
	if err := infra.GetStackManager().DeleteStack(ctx, id, os.Stdout); err != nil {
		return fmt.Errorf("failed to destroy stack %q: %w", id, err)
	}
	if err := deleteRecord(id); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not remove environment record: %v\n", err)
	}
	fmt.Printf("Environment %q deleted.\n", id)
	return nil
}

func agentKey(name string) string {
	return commonconfig.DDAgentConfigNamespace + ":" + name
}

// extractHostIP reads the host IP from the stack outputs, mirroring the former
// Python logic: prefer the explicit "hostIP" output, then fall back to the
// standard e2e-framework "dd-Host-<name>".address shape.
func extractHostIP(up auto.UpResult) string {
	if v, ok := up.Outputs["hostIP"]; ok {
		if s, ok := v.Value.(string); ok && s != "" {
			return s
		}
	}
	for key, out := range up.Outputs {
		if !strings.HasPrefix(key, "dd-Host-") {
			continue
		}
		if m, ok := out.Value.(map[string]interface{}); ok {
			if addr, ok := m["address"].(string); ok && addr != "" {
				return addr
			}
		}
	}
	return ""
}
