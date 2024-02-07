// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ebpf

import (
	"fmt"
	"io"

	manager "github.com/DataDog/ebpf-manager"
	"golang.org/x/exp/slices"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Manager wraps ebpf-manager.Manager, adding a property with the list of enabled modifiers
// for this instance.
type Manager struct {
	*manager.Manager
	EnabledModifiers []string // List of enabled modifiers
}

// Names of modifiers are defined here to avoid duplication and so that they can be
// used in the list of default modifiers
const (
	// NoModifiers is a constant to be used when creating a new manager to avoid initializing the default modifiers
	NoModifiers = "NO_MODIFIERS"
	// PrintkModifier is the name of the modifier that patches the printk function
	PrintkModifier = "printk"
)

// defaultModifiers is a list of modifiers that are enabled by default when the callers don't provide
// a specific list
var defaultModifiers = []string{PrintkModifier}

// NewManager creates a manager wrapper.
// If the modifiers list is empty, it will be initialized with the default modifiers.
// Pass manager.NO_MODIFIERS to avoid initializing the default modifiers.
func NewManager(mgr *manager.Manager, modifiers ...string) *Manager {
	if len(modifiers) == 0 {
		modifiers = defaultModifiers
	} else if len(modifiers) == 1 && modifiers[0] == NoModifiers {
		modifiers = []string{}
	}

	log.Debugf("Creating new manager with modifiers: %v", modifiers)

	return &Manager{
		Manager:          mgr,
		EnabledModifiers: modifiers,
	}
}

// Modifier is an interface that can be implemented by a package to
// add functionality to the ebpf.Manager. It exposes a name to identify the modifier,
// two functions that will be called before and after the ebpf.Manager.InitWithOptions
// call, and a function that will be called when the manager is stopped.
type Modifier interface {
	// Name returns the name of the modifier. Should be unique, although it's not enforced for now.
	Name() string

	// BeforeInit is called before the ebpf.Manager.InitWithOptions call
	BeforeInit(*Manager, *manager.Options) error

	// AfterInit is called after the ebpf.Manager.InitWithOptions call
	AfterInit(*Manager, *manager.Options) error
}

// Internal state with all registered modifiers. This is populated via the
// RegisterModifier function below by packages that want to add a modifier.
var modifiers []Modifier

// RegisterModifier registers a Modifier to be run whenever a new manager is
// initialized. This is used to add functionality to the manager, such as telemetry or
// the newline patching
// This should be called on init() functions of packages that want to add a modifier.
func RegisterModifier(mod Modifier) {
	modifiers = append(modifiers, mod)
}

// enabledModifiers is a shorthand to return a list of all enabled modifiers
// for this manager
func (m *Manager) getEnabledModifiers() []Modifier {
	var enabled []Modifier
	for _, mod := range modifiers {
		if slices.Contains(m.EnabledModifiers, mod.Name()) {
			enabled = append(enabled, mod)
		}
	}
	return enabled
}

// InitWithOptions is a wrapper around ebpf-manager.Manager.InitWithOptions
func (m *Manager) InitWithOptions(bytecode io.ReaderAt, opts *manager.Options) error {
	for _, mod := range m.getEnabledModifiers() {
		log.Debugf("Running %s manager modifier", mod.Name())
		if err := mod.BeforeInit(m, opts); err != nil {
			return fmt.Errorf("error running %s manager modifier: %w", mod.Name(), err)
		}
	}

	if err := m.Manager.InitWithOptions(bytecode, *opts); err != nil {
		return err
	}

	for _, mod := range m.getEnabledModifiers() {
		log.Debugf("Running %s manager modifier", mod.Name())
		if err := mod.AfterInit(m, opts); err != nil {
			return fmt.Errorf("error running %s manager modifier: %w", mod.Name(), err)
		}
	}
	return nil
}
