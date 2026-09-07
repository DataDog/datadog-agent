// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package driver adapts typed driver implementations into an explicit catalog.
// Define supplies automatic validation/defaulting/example generation; drivers
// implement lifecycle behavior and a required description, not schema plumbing.
package driver

import (
	"fmt"
	"sort"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/envstore"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/installer"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/configschema"
)

// Implementation is the typed extension contract. Validate(P) error is optional;
// when implemented it runs after schema validation and defaults. Pure semantic
// rules shared with an executor should instead be registered on the shared Schema.
type Implementation[P any] interface {
	ID() string
	Description() string
	Start(P, *config.File, envstore.Entry, *envstore.Store) error
	Stop(P, *config.File, envstore.Entry, *envstore.Store) error
	Installers() []installer.Installer
}

// Driver is the type-erased catalog entry, implemented by Define, not by drivers.
type Driver interface {
	ID() string
	Description() string
	Installers() []installer.Installer
	Prepare(*config.File) (*Prepared, error)
	StarterConfig() ([]byte, error)
	Stop(envstore.Entry, *envstore.Store) error
}

// Prepared captures typed input so validation and execution use the same value.
type Prepared struct {
	Config *config.File
	start  func(envstore.Entry, *envstore.Store) error
}

func (p *Prepared) Start(entry envstore.Entry, store *envstore.Store) error {
	return p.start(entry, store)
}

type typedDriver[P any] struct {
	schema           *configschema.Schema[P]
	defaultInstaller string
	impl             Implementation[P]
}

// Define is called explicitly by registry.go. Schemas are compiled once, and
// descriptions/default installers are checked before exposing a catalog entry.
func Define[P any](schema *configschema.Schema[P], defaultInstaller string, impl Implementation[P]) Driver {
	if schema == nil || strings.TrimSpace(impl.ID()) == "" || strings.TrimSpace(impl.Description()) == "" {
		panic("driver registration requires a schema, ID and description")
	}
	d := &typedDriver[P]{schema: schema, defaultInstaller: defaultInstaller, impl: impl}
	if _, err := InstallerFor(d, defaultInstaller); err != nil {
		panic(err)
	}
	return d
}

func (d *typedDriver[P]) ID() string                        { return d.impl.ID() }
func (d *typedDriver[P]) Description() string               { return d.impl.Description() }
func (d *typedDriver[P]) Installers() []installer.Installer { return d.impl.Installers() }

func (d *typedDriver[P]) Prepare(cfg *config.File) (*Prepared, error) {
	if cfg.Environment.Base != d.ID() {
		return nil, fmt.Errorf("driver %q cannot prepare base %q", d.ID(), cfg.Environment.Base)
	}
	path := "environment." + d.ID()
	if cfg.Path != "" {
		path = cfg.Path + ": " + path
	}
	var params P
	var raw []byte
	var err error
	if cfg.Environment.SectionNode != nil {
		params, raw, err = d.schema.DecodeNode(cfg.Environment.SectionNode, path)
	} else {
		params, raw, err = d.schema.Decode(cfg.Environment.Section, path)
	}
	if err != nil {
		return nil, err
	}
	if v, ok := d.impl.(configschema.Validator[P]); ok {
		if err := v.Validate(params); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
	}
	normal := *cfg
	normal.Environment.Section = raw
	normal.Environment.SectionNode = nil
	return &Prepared{
		Config: &normal,
		start: func(entry envstore.Entry, store *envstore.Store) error {
			return d.impl.Start(params, &normal, entry, store)
		},
	}, nil
}

func (d *typedDriver[P]) Stop(entry envstore.Entry, store *envstore.Store) error {
	cfg, err := entry.LoadConfig()
	if err != nil {
		return err
	}
	params, raw, err := d.schema.Decode(cfg.Environment.Section, "environment."+d.ID())
	if err != nil {
		return err
	}
	// Teardown does not invoke optional runtime-driver semantic checks: it must
	// remain possible to delete a target after an installation prerequisite changes.
	cfg.Environment.Section = raw
	cfg.Environment.SectionNode = nil
	return d.impl.Stop(params, cfg, entry, store)
}

func (d *typedDriver[P]) StarterConfig() ([]byte, error) {
	section, err := d.schema.Example(nil)
	if err != nil {
		return nil, fmt.Errorf("generating %q example: %w", d.ID(), err)
	}
	if v, ok := d.impl.(configschema.Validator[P]); ok {
		var params P
		if err := section.Decode(&params); err != nil {
			return nil, err
		}
		if err := v.Validate(params); err != nil {
			return nil, fmt.Errorf("invalid %q example: %w", d.ID(), err)
		}
	}
	data, err := config.Example(d.ID(), d.Description(), d.defaultInstaller, section)
	if err != nil {
		return nil, err
	}
	cfg, errs := config.Parse(data)
	if err := config.NewErrors(errs); err != nil {
		return nil, err
	}
	inst, err := InstallerFor(d, cfg.Agent.Install)
	if err != nil {
		return nil, err
	}
	if err := config.NewErrors(inst.Validate(cfg)); err != nil {
		return nil, fmt.Errorf("invalid installer example: %w", err)
	}
	return data, nil
}

func Get(base string) (Driver, error) {
	for _, d := range registry {
		if d.ID() == base {
			return d, nil
		}
	}
	return nil, fmt.Errorf("environment.base %q is not supported (registered: %s)", base, IDs())
}

func IDs() []string {
	ids := make([]string, 0, len(registry))
	for _, d := range registry {
		ids = append(ids, d.ID())
	}
	sort.Strings(ids)
	return ids
}

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
	return nil, fmt.Errorf("agent.install %q is not supported for base %q (supported: %s)", id, d.ID(), strings.Join(ids, ", "))
}
