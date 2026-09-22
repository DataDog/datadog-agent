// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package pulumiworker holds the lifecycle shared by every driver whose
// infrastructure is provisioned by the e2ectl-worker (the Pulumi executor):
// start runs the executor's provision action, reads the exported fakeintake
// endpoint and marks the environment ready; stop destroys the stack and
// removes the entry. Scenario drivers embed Lifecycle and add only what is
// specific to their base (for example exporting a cluster kubeconfig after
// provisioning), so a new cloud base is a registration plus its differences —
// not a third copy of the executor plumbing.
package pulumiworker

import (
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/envstore"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/installer"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/workerclient"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/outputs"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioner"
)

// PostProvision runs after a successful provision and before the environment
// is marked ready. It reads the snapshot (the single source of truth) and may
// enrich the entry or its meta — for example a cluster base exporting its
// kubeconfig next to the entry. It must not mutate the provisioned
// infrastructure.
type PostProvision func(entry envstore.Entry, meta *envstore.Meta) error

// Lifecycle is the shared behavior half of a Pulumi-executor driver. The typed
// half lives in the scenario driver: it implements the typed Start/Stop
// signatures by forwarding to Start/Stop here (Go methods cannot take the
// config type parameter, so the forwarding is one line per lifecycle method).
// The fields stay unexported — ID(), Description() and Installers() are the
// interface methods over them — and New is the single constructor.
type Lifecycle struct {
	base          string
	label         string
	description   string
	installers    []installer.Installer
	postProvision PostProvision
}

// New declares one Pulumi-executor lifecycle: base is the environment base
// (the driver ID, the config section name and the workerclient base — one
// spelling everywhere), label the human name used in progress messages, and
// postProvision may be nil for plain host bases.
func New(base, label, description string, installers []installer.Installer, postProvision PostProvision) *Lifecycle {
	return &Lifecycle{
		base:          base,
		label:         label,
		description:   description,
		installers:    installers,
		postProvision: postProvision,
	}
}

func (l *Lifecycle) ID() string          { return l.base }
func (l *Lifecycle) Description() string { return l.description }
func (l *Lifecycle) Installers() []installer.Installer {
	return l.installers
}

// Start provisions through the executor. Failure marking (status=error) is
// owned by the command layer, not repeated here: cmdStart re-reads the entry
// and marks it failed whenever Start returns an error.
func (l *Lifecycle) Start(cfg *config.File, entry envstore.Entry, store *envstore.Store) error {
	fmt.Printf("provisioning %s (Pulumi executor), this takes a few minutes...\n", l.label)
	if err := workerclient.Run(entry.Dir, l.Job(workerclient.ActionProvision, cfg, entry)); err != nil {
		return err
	}
	if err := ReadFakeintakeOutput(entry, cfg.FakeIntakeEnabled(), &entry.Meta); err != nil {
		return err
	}
	if l.postProvision != nil {
		if err := l.postProvision(entry, &entry.Meta); err != nil {
			return err
		}
	}
	entry.Meta.Status = envstore.StatusReady
	return store.UpdateMeta(entry)
}

// Stop destroys the stack through the executor and removes the entry.
func (l *Lifecycle) Stop(cfg *config.File, entry envstore.Entry, store *envstore.Store) error {
	fmt.Printf("destroying %s (Pulumi executor)...\n", l.label)
	if err := workerclient.Run(entry.Dir, l.Job(workerclient.ActionDestroy, cfg, entry)); err != nil {
		return err
	}
	return store.Delete(entry.Name)
}

// Job builds the versioned executor request from the normalized driver-owned
// config section: unlike re-marshalling with omitempty, forwarding the
// normalized YAML preserves explicit zeros and applied defaults for any
// future fields. No second DTO is declared per base.
func (l *Lifecycle) Job(action string, cfg *config.File, entry envstore.Entry) workerclient.Job {
	fixtures := cfg.Environment.Fixtures
	return workerclient.Job{
		ProtocolVersion: workerclient.ProtocolVersion,
		Action:          action,
		Base:            l.base,
		Params:          string(cfg.Environment.Section),
		Fixtures:        &fixtures,
		StackName:       StackName(entry.Name),
		EnvDir:          entry.Dir,
	}
}

// ReadFakeintakeOutput uses the endpoint exported by the Pulumi scenario,
// including its resource binding. It does not connect to or mutate the target.
func ReadFakeintakeOutput(entry envstore.Entry, enabled bool, meta *envstore.Meta) error {
	if !enabled {
		meta.FakeIntakeURL = ""
		meta.FakeIntakePort = 0
		return nil
	}
	var fi outputs.FakeintakeOutput
	if err := provisioner.ReadSnapshotResource(entry.SnapshotPath(), "fakeIntake", &fi); err != nil {
		return fmt.Errorf("reading Pulumi-provisioned fakeintake: %w", err)
	}
	if fi.URL == "" {
		return fmt.Errorf("snapshot %s has no URL for the Pulumi-provisioned fakeintake", entry.SnapshotPath())
	}
	// Cloud fakeintakes (ECS Fargate) run in the same VPC as the agent and
	// the operator — the URL is the same from both. The Pulumi component sets
	// only `url` at construction time; the agent/query split is a local-Docker
	// distinction (container DNS name vs published host port).
	if fi.AgentURL == "" {
		fi.AgentURL = fi.URL
	}
	if fi.QueryURL == "" {
		fi.QueryURL = fi.URL
	}
	if err := provisioner.UpdateSnapshotResource(entry.SnapshotPath(), "fakeIntake", mustMarshal(fi)); err != nil {
		return err
	}
	meta.FakeIntakeURL = fi.URL
	meta.FakeIntakePort = int(fi.Port)
	return nil
}

var stackNameRegexp = regexp.MustCompile(`[^a-zA-Z0-9-_.]`)

// StackName derives the Pulumi stack for an environment: executor-wide
// bookkeeping, deterministic so destroy finds what provision created.
func StackName(name string) string {
	return "e2ectl-" + stackNameRegexp.ReplaceAllString(name, "-")
}

func mustMarshal(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err) // outputs.FakeintakeOutput is always marshalable
	}
	return data
}
