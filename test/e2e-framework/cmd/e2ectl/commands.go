// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present, Datadog, Inc.

package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/envstore"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/fakeintakecmd"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/kinddriver"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/workerclient"
)

func cmdStart(args []string) error {
	fs := flag.NewFlagSet("start", flag.ContinueOnError)
	configPath := fs.String("config", "", "environment config file (required)")
	name := fs.String("name", "", "environment name (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *configPath == "" || *name == "" {
		return fmt.Errorf("both --config and --name are required")
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	store, err := envstore.New()
	if err != nil {
		return err
	}
	entry, err := store.Create(*name, cfg, envstore.Meta{})
	if err != nil {
		return err
	}

	switch cfg.Environment.Base {
	case config.BaseKind:
		if err := kinddriver.Start(cfg, entry, store); err != nil {
			entry.Meta.Status = envstore.StatusError
			_ = store.UpdateMeta(entry)
			return err
		}
		fmt.Printf("environment %q is ready (kind cluster, kubeconfig: %s)\n",
			*name, entry.KubeconfigPath())
		if cfg.FakeIntakeEnabled() {
			e, _ := store.Get(*name)
			fmt.Printf("fakeintake: %s\n", e.Meta.FakeIntakeURL)
		}
		return nil
	case config.BaseEC2Host:
		entry.Meta.StackName = "e2ectl-" + sanitizeStackName(*name)
		_ = store.UpdateMeta(entry)
		fmt.Printf("provisioning EC2 host (Pulumi), this takes a few minutes...\n")
		err := workerclient.Run(entry.Dir, workerclient.Job{
			Action:       "provision-ec2",
			EnvDir:       entry.Dir,
			StackName:    entry.Meta.StackName,
			OS:           cfg.Environment.VM.OS,
			Arch:         cfg.Environment.VM.Arch,
			InstanceType: cfg.Environment.VM.InstanceType,
			FakeIntake:   cfg.FakeIntakeEnabled(),
		})
		if err != nil {
			entry.Meta.Status = envstore.StatusError
			_ = store.UpdateMeta(entry)
			return err
		}
		entry.Meta.Status = envstore.StatusReady
		if err := fillFakeintakeURL(entry, &entry.Meta); err != nil {
			return err
		}
		return store.UpdateMeta(entry)
	default:
		return fmt.Errorf("unsupported base %q", cfg.Environment.Base)
	}
}

// fillFakeintakeURL reads the fakeintake URL from the snapshot so metadata
// works for both drivers.
func fillFakeintakeURL(entry envstore.Entry, meta *envstore.Meta) error {
	resources, _, err := readSnapshot(entry.SnapshotPath())
	if err != nil {
		return err
	}
	var fi struct {
		Host string `json:"host"`
		Port int    `json:"port"`
		URL  string `json:"url"`
	}
	if raw, ok := resources["fakeIntake"]; ok {
		if err := jsonUnmarshal(raw, &fi); err != nil {
			return err
		}
		meta.FakeIntakeURL = fi.URL
	}
	return nil
}

func cmdList(args []string) error {
	store, err := envstore.New()
	if err != nil {
		return err
	}
	entries, err := store.List()
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		fmt.Println("no environments (e2ectl start --config <env>.yml --name <name>)")
		return nil
	}
	fmt.Printf("%-20s %-10s %-9s %-8s %s\n", "NAME", "BASE", "STATUS", "AGE", "AGENT")
	for _, e := range entries {
		agent := "-"
		if e.Meta.AgentInstalled {
			if e.Meta.AgentImage != "" {
				agent = e.Meta.AgentImage
			} else {
				agent = e.Meta.AgentVersion
			}
		}
		fmt.Printf("%-20s %-10s %-9s %-8s %s\n",
			e.Name, e.Meta.Base, e.Meta.Status, age(e.Meta.CreatedAt), agent)
	}
	return nil
}

func cmdInstall(args []string) error {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	configPath := fs.String("config", "", "environment config file (required)")
	name := fs.String("env", "", "environment name (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *configPath == "" || *name == "" {
		return fmt.Errorf("both --config and --env are required")
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	store, err := envstore.New()
	if err != nil {
		return err
	}
	entry, err := store.Get(*name)
	if err != nil {
		return err
	}
	if entry.Meta.Status != envstore.StatusReady {
		return fmt.Errorf("environment %q is not ready (status: %s)", *name, entry.Meta.Status)
	}
	if entry.Meta.Base != cfg.Environment.Base {
		return fmt.Errorf("config base %q does not match environment base %q",
			cfg.Environment.Base, entry.Meta.Base)
	}

	var action string
	switch cfg.Environment.Base {
	case config.BaseKind:
		action = "install-kind"
	case config.BaseEC2Host:
		action = "install-host"
	default:
		return fmt.Errorf("unsupported base %q", cfg.Environment.Base)
	}
	err = workerclient.Run(entry.Dir, workerclient.Job{
		Action:       action,
		EnvDir:       entry.Dir,
		Version:      cfg.Agent.Version,
		Image:        cfg.Agent.Image,
		AgentConfig:  cfg.Agent.Config,
		Integrations: cfg.Agent.Integrations,
	})
	if err != nil {
		return err
	}
	entry.Meta.AgentInstalled = true
	entry.Meta.AgentVersion = cfg.Agent.Version
	entry.Meta.AgentImage = cfg.Agent.Image
	return store.UpdateMeta(entry)
}

func cmdUpdate(args []string) error {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	name := fs.String("env", "", "environment name (required)")
	skipBuild := fs.Bool("skip-build", false, "do not rebuild the agent image; reuse the one referenced in the config")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *name == "" {
		return fmt.Errorf("--env is required")
	}
	store, err := envstore.New()
	if err != nil {
		return err
	}
	entry, err := store.Get(*name)
	if err != nil {
		return err
	}
	if entry.Meta.Base != config.BaseKind {
		return fmt.Errorf("update only supports kind environments so far (this one is %q)", entry.Meta.Base)
	}
	cfg, err := entry.LoadConfig()
	if err != nil {
		return err
	}
	if cfg.Agent.Image == "" {
		return fmt.Errorf("update requires agent.image in the environment config (e.g. gcr.io/datadoghq/agent:my-dev)")
	}

	if !*skipBuild {
		fmt.Printf("building agent image %s (dda inv agent.hacky-dev-image-build)...\n", cfg.Agent.Image)
		if err := buildAgentImage(cfg.Agent.Image); err != nil {
			return err
		}
	}

	if err := workerclient.Run(entry.Dir, workerclient.Job{
		Action: "update-kind",
		EnvDir: entry.Dir,
		Image:  cfg.Agent.Image,
	}); err != nil {
		return err
	}
	entry.Meta.AgentInstalled = true
	entry.Meta.AgentImage = cfg.Agent.Image
	return store.UpdateMeta(entry)
}

// buildAgentImage runs the repo's dev image build, tagging the result exactly
// as the config references it.
func buildAgentImage(image string) error {
	cmd := osCommand("dda", "inv", "agent.hacky-dev-image-build", "--target-image="+image)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func cmdFakeintake(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: e2ectl fakeintake <names|metrics|health> --env <name> [--name <metric>] [--json]")
	}
	sub, rest := args[0], args[1:]
	fs := flag.NewFlagSet("fakeintake "+sub, flag.ContinueOnError)
	name := fs.String("env", "", "environment name (required)")
	metric := fs.String("name", "", "metric name (metrics subcommand)")
	asJSON := fs.Bool("json", false, "JSON output")
	if err := fs.Parse(rest); err != nil {
		return err
	}
	if *name == "" {
		return fmt.Errorf("--env is required")
	}
	store, err := envstore.New()
	if err != nil {
		return err
	}
	entry, err := store.Get(*name)
	if err != nil {
		return err
	}
	switch sub {
	case "names":
		return fakeintakecmd.Names(entry.Meta.FakeIntakeURL, *asJSON)
	case "metrics":
		return fakeintakecmd.Metrics(entry.Meta.FakeIntakeURL, *metric, *asJSON)
	case "health":
		return fakeintakecmd.Health(entry.Meta.FakeIntakeURL)
	default:
		return fmt.Errorf("unknown fakeintake subcommand %q (names|metrics|health)", sub)
	}
}

func cmdStop(args []string) error {
	fs := flag.NewFlagSet("stop", flag.ContinueOnError)
	name := fs.String("env", "", "environment name (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *name == "" {
		return fmt.Errorf("--env is required")
	}
	store, err := envstore.New()
	if err != nil {
		return err
	}
	entry, err := store.Get(*name)
	if err != nil {
		return err
	}
	switch entry.Meta.Base {
	case config.BaseKind:
		return kinddriver.Stop(entry, store)
	case config.BaseEC2Host:
		cfg, err := entry.LoadConfig()
		if err != nil {
			return err
		}
		fmt.Println("destroying EC2 host (Pulumi)...")
		if err := workerclient.Run(entry.Dir, workerclient.Job{
			Action:       "destroy-ec2",
			EnvDir:       entry.Dir,
			StackName:    entry.Meta.StackName,
			OS:           cfg.Environment.VM.OS,
			Arch:         cfg.Environment.VM.Arch,
			InstanceType: cfg.Environment.VM.InstanceType,
			FakeIntake:   cfg.FakeIntakeEnabled(),
		}); err != nil {
			return err
		}
		return store.Delete(*name)
	default:
		return fmt.Errorf("unsupported base %q", entry.Meta.Base)
	}
}

func age(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func sanitizeStackName(name string) string {
	out := make([]rune, 0, len(name))
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			out = append(out, r)
		default:
			out = append(out, '-')
		}
	}
	return string(out)
}
