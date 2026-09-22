// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package local drives the developer's host as an environment: a per-env
// Docker network, the fakeintake container on it, and nothing else — the
// Agent is installed later, in a container on the same network. Pulumi-free
// like kind; deterministic names make failed starts recoverable with plain
// `stop`.
package local

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/envstore"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/installer"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/localinfra"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/receiver"
	localconfig "github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/envconfig/local"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioner"
)

type Driver struct{}

func (d *Driver) ID() string { return "local" }
func (d *Driver) Description() string {
	return "The local host as the environment: Agent in a container, fakeintake in Docker"
}

func (d *Driver) Installers() []installer.Installer {
	return []installer.Installer{&installer.Binary{}, &installer.Package{}}
}

// Start always creates the producer network, with an optional capture fixture.
func (d *Driver) Start(_ localconfig.Config, cfg *config.File, entry envstore.Entry, store *envstore.Store) error {
	network := localinfra.NetworkName(entry.Name)
	if err := localinfra.CreateNetwork(network); err != nil {
		return fmt.Errorf("creating docker network %s: %w", network, err)
	}
	meta := entry.Meta
	resources := provisioner.RawResources{}
	if cfg.FakeIntakeEnabled() {
		port, err := localinfra.RunFakeintakeOnNetwork(localinfra.FakeintakeContainer(entry.Name), network)
		if err != nil {
			return err
		}

		meta.FakeIntakePort = port
		meta.FakeIntakeURL = fmt.Sprintf("http://127.0.0.1:%d", port)
		fiKey, err := json.Marshal(map[string]any{
			"host":     "127.0.0.1",
			"scheme":   "http",
			"port":     port,
			"url":      meta.FakeIntakeURL,
			"queryURL": meta.FakeIntakeURL,
			"agentURL": "http://" + localinfra.FakeintakeContainer(entry.Name) + ":80",
		})
		if err != nil {
			return err
		}
		resources["fakeIntake"] = fiKey
	}
	meta.Status = envstore.StatusReady
	if err := provisioner.WriteSnapshotFile(entry.SnapshotPath(), resources, map[string]any{
		"source": "e2ectl-local",
	}); err != nil {
		return err
	}
	entry.Meta = meta
	return store.UpdateMeta(entry)
}

// Stop removes the agent container (best-effort — install may never have run),
// its exact owned runtime volume, the managed blackhole sink, fakeintake and
// network, then the entry. All names are deterministic, so a half-created
// environment is always recoverable; the stop --force escape hatch exists but
// plain stop must already work.
func (d *Driver) Stop(_ localconfig.Config, _ *config.File, entry envstore.Entry, store *envstore.Store) error {
	unlock, err := installer.LockAgentOperation(entry)
	if err != nil {
		return err
	}
	defer unlock()
	if _, err := os.Stat(entry.SnapshotPath()); os.IsNotExist(err) && !entry.Meta.AgentInstalled {
		// Start never published an environment, so no installer could create
		// runtime state. Preserve offline recovery from a missing Docker tool.
		_ = localinfra.RemoveContainer(localinfra.AgentContainer(entry.Name))
	} else if err := localinfra.RemoveAgentAndRuntime(localinfra.AgentContainer(entry.Name), localinfra.AgentRuntimeVolume(entry.Dir, entry.Meta.CreatedAt), nil); err != nil {
		return err
	}
	_ = localinfra.StopFakeintake(localinfra.FakeintakeContainer(entry.Name))
	_ = localinfra.StopBlackhole(localinfra.BlackholeContainer(entry.Name))
	_ = localinfra.RemoveNetwork(localinfra.NetworkName(entry.Name))
	return store.Delete(entry.Name)
}

// SyncManagedSink implements installer.ManagedSinkSyncer: `receiver: type:
// blackhole` with no url means the environment provides the sink itself. The
// managed sink exists only while an agent is routed to it — install/apply sync
// it before the agent container starts, and any other selection removes it.
// The container is unprivileged, read-only, publishes no host port, and is
// named deterministically so `stop` always cleans it up.
func (d *Driver) SyncManagedSink(cfg *config.File, entry envstore.Entry) error {
	container := localinfra.BlackholeContainer(entry.Name)
	if !receiver.ManagedSinkRequired(cfg.Agent.Receiver) {
		// Not a managed sink selection: remove a previous managed sink and
		// tombstone its snapshot fact so stale URLs never resolve later.
		_ = localinfra.StopBlackhole(container)
		return provisioner.UpdateSnapshotResources(entry.SnapshotPath(), provisioner.RawResources{"blackhole": json.RawMessage("null")}, nil)
	}
	binary, err := localinfra.StageBlackholeBinary(entry.Dir)
	if err != nil {
		return err
	}
	// Replace any previous sink first: the sink is stateless, and the agent
	// container that follows must route to this exact, freshly started one.
	_ = localinfra.StopBlackhole(container)
	if err := localinfra.RunBlackholeOnNetwork(container, localinfra.NetworkName(entry.Name), installer.DefaultRuntimeImage, binary); err != nil {
		return err
	}
	out, err := json.Marshal(receiver.BlackholeOutput{AgentURL: localinfra.BlackholeAgentURL(entry.Name)})
	if err != nil {
		return err
	}
	return provisioner.UpdateSnapshotResources(entry.SnapshotPath(), provisioner.RawResources{"blackhole": out}, nil)
}
