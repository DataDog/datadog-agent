// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product contains software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present, Datadog, Inc.

// Package driver defines the extension contract of e2ectl: a Driver owns one
// environment type (a "base"). Adding an environment means implementing
// these interfaces in one package and registering the driver in registry.go
// — no edits to any command, the config core, the envstore or the worker.
//
// Driver knowledge belongs to the driver: the generic layers (commands,
// envstore, worker jobs) carry opaque payloads the driver decodes itself.
package driver

import (
	"fmt"
	"sort"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/envstore"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/installer"
)

// Driver owns one environment type.
type Driver interface {
	// ID is the environment.base value and the config section name.
	ID() string
	// Validate strict-decodes and validates the driver's own config section
	// (cfg.Environment.Section). Generic fields are already validated.
	Validate(cfg *config.File) []error
	// Start provisions the environment and writes its snapshot. Fakeintake
	// follows the driver's provisioning path: Pulumi for cloud scenarios,
	// local Docker for kind, when cfg.FakeIntakeEnabled().
	Start(cfg *config.File, entry envstore.Entry, store *envstore.Store) error
	// Stop destroys the environment and removes the entry.
	Stop(entry envstore.Entry, store *envstore.Store) error
	// Installers lists the agent install methods this base supports.
	Installers() []installer.Installer
}

// Get returns the driver for a base, with an error listing the registered ones.
func Get(base string) (Driver, error) {
	for _, d := range registry {
		if d.ID() == base {
			return d, nil
		}
	}
	return nil, fmt.Errorf("environment.base %q is not supported (registered: %s)", base, IDs())
}

// IDs returns the registered base IDs, sorted.
func IDs() []string {
	ids := make([]string, 0, len(registry))
	for _, d := range registry {
		ids = append(ids, d.ID())
	}
	sort.Strings(ids)
	return ids
}

// InstallerFor returns the driver's installer with the given id.
func InstallerFor(d Driver, id string) (installer.Installer, error) {
	for _, i := range d.Installers() {
		if i.ID() == id {
			return i, nil
		}
	}
	ids := make([]string, 0, len(d.Installers()))
	for _, i := range d.Installers() {
		ids = append(ids, i.ID())
	}
	return nil, fmt.Errorf("agent.install %q is not supported for base %q (supported: %s)",
		id, d.ID(), strings.Join(ids, ", "))
}
