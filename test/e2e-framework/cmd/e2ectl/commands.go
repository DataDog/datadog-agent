// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package main is the e2ectl CLI: the fast, Pulumi-free surface. The commands
// are registry-driven — there is no switch on environment type anywhere;
// each command looks up the driver and calls it.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/driver"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/envstore"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/fakeintakecmd"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/installer"
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
	d, err := driver.Get(cfg.Environment.Base)
	if err != nil {
		return err
	}
	prepared, err := d.Prepare(cfg)
	if err != nil {
		return err
	}
	cfg = prepared.Config
	store, err := envstore.New()
	if err != nil {
		return err
	}
	entry, err := store.Create(*name, cfg, envstore.Meta{})
	if err != nil {
		return err
	}

	if err := prepared.Start(entry, store); err != nil {
		return err
	}

	// the generic post-start report: everything below comes from common meta
	// or well-known snapshot keys, so no driver is consulted
	e, _ := store.Get(*name)
	fmt.Printf("environment %q is %s\n", *name, e.Meta.Status)
	if fi := e.Meta.FakeIntakeURL; fi != "" {
		fmt.Printf("fakeintake: %s\n", fi)
	}
	if kubeconfig := e.KubeconfigPath(); e.Meta.Base == "kind" || fileExists(kubeconfig) {
		_ = kubeconfig // cluster drivers report the kubeconfig themselves
	}
	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
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
	configPath := fs.String("config", "", "environment config file (defaults to the stored config)")
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
	if entry.Meta.Status != envstore.StatusReady {
		return fmt.Errorf("environment %q is not ready (status: %s)", *name, entry.Meta.Status)
	}
	cfg, err := loadOrStoredConfig(*configPath, entry)
	if err != nil {
		return err
	}
	d, err := driver.Get(cfg.Environment.Base)
	if err != nil {
		return err
	}
	if d.ID() != entry.Meta.Base {
		return fmt.Errorf("config base %q does not match environment base %q", d.ID(), entry.Meta.Base)
	}
	inst, err := driver.InstallerFor(d, cfg.Agent.Install)
	if err != nil {
		return err
	}
	if errs := inst.Validate(cfg); len(errs) > 0 {
		return config.NewErrors(errs)
	}

	if err := inst.Install(cfg, entry); err != nil {
		return err
	}
	if err := saveAppliedConfig(cfg, entry); err != nil {
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
	configPath := fs.String("config", "", "environment config file; replaces the stored config copy (e.g. to change the agent image)")
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
	cfg, err := loadOrStoredConfig(*configPath, entry)
	if err != nil {
		return err
	}
	d, err := driver.Get(cfg.Environment.Base)
	if err != nil {
		return err
	}
	if d.ID() != entry.Meta.Base {
		return fmt.Errorf("config base %q does not match environment base %q", d.ID(), entry.Meta.Base)
	}
	inst, err := driver.InstallerFor(d, cfg.Agent.Install)
	if err != nil {
		return err
	}
	updatable, ok := inst.(installer.Updatable)
	if !ok {
		return fmt.Errorf("update is not supported for base %q with install %q yet", d.ID(), inst.ID())
	}
	if errs := inst.Validate(cfg); len(errs) > 0 {
		return config.NewErrors(errs)
	}

	if !*skipBuild {
		fmt.Printf("building agent image %s (dda inv agent.hacky-dev-image-build)...\n", cfg.Agent.Image)
		if err := buildAgentImage(cfg.Agent.Image); err != nil {
			return err
		}
	}

	if err := updatable.Update(cfg, entry); err != nil {
		return err
	}
	if err := saveAppliedConfig(cfg, entry); err != nil {
		return err
	}
	entry.Meta.AgentInstalled = true
	entry.Meta.AgentImage = cfg.Agent.Image
	return store.UpdateMeta(entry)
}

// Parse and prepare without writing: schema errors and optional Validate(params)
// failures must not replace the stored config or touch the environment.
func loadOrStoredConfig(path string, entry envstore.Entry) (*config.File, error) {
	if path == "" {
		path = entry.ConfigPath()
	}
	cfg, err := config.Load(path)
	if err != nil {
		return nil, err
	}
	if cfg.Environment.Base != entry.Meta.Base {
		return nil, fmt.Errorf("config base %q does not match environment base %q", cfg.Environment.Base, entry.Meta.Base)
	}
	d, err := driver.Get(cfg.Environment.Base)
	if err != nil {
		return nil, err
	}
	prepared, err := d.Prepare(cfg)
	if err != nil {
		return nil, err
	}
	return prepared.Config, nil
}

func saveAppliedConfig(cfg *config.File, entry envstore.Entry) error {
	data := cfg.Source()
	if len(data) == 0 {
		return fmt.Errorf("cannot persist config without its parsed source")
	}
	f, err := os.CreateTemp(entry.Dir, ".config-*.yaml")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), entry.ConfigPath())
}

// buildAgentImage runs the repo's dev image build, tagging the result exactly
// as the config references it.
func buildAgentImage(image string) error {
	cmd := exec.Command("dda", "inv", "agent.hacky-dev-image-build", "--target-image="+image)
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
	d, err := driver.Get(entry.Meta.Base)
	if err != nil {
		return err
	}
	return d.Stop(entry, store)
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
