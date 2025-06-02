// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ebpf

import (
	"errors"
	"fmt"
	"io"
	"sync"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/ebpf/names"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Manager wraps ebpf-manager.Manager, adding a property with the list of enabled modifiers
// for this instance.
type Manager struct {
	*manager.Manager
	Name             names.ModuleName
	EnabledModifiers []Modifier // List of enabled modifiers
}

var modifiersSync sync.Once

// defaultModifiers is a list of modifiers that are enabled by default when the manager wrapper is initialized using the relevant ctor (see NewManagerWithDefault).
// This is a static list lazy-initialized once during the lifetime of the program, hence the modifiers in this list must be stateless.
var defaultModifiers []Modifier

// NewManager creates a manager wrapper.
// Optionally one can provide a list of modifiers to attach to the manager
func NewManager(mgr *manager.Manager, name string, modifiers ...Modifier) *Manager {
	log.Tracef("Creating new manager with modifiers: %v", modifiers)
	return &Manager{
		Manager:          mgr,
		Name:             names.NewModuleName(name),
		EnabledModifiers: modifiers,
	}
}

// NewManagerWithDefault creates a manager wrapper with default modifiers.
func NewManagerWithDefault(mgr *manager.Manager, name string, modifiers ...Modifier) *Manager {
	modifiersSync.Do(func() {
		defaultModifiers = []Modifier{&PrintkPatcherModifier{}}
	})
	return NewManager(mgr, name, append(defaultModifiers, modifiers...)...)
}

// Modifier is an interface that can be implemented by a package to add
// functionality to the ebpf.Manager. It exposes a name to identify the
// modifier, and then any of the functions Before/AfterInit, Before/AfterStart,
// Before/AfterStop, that will be called at the corresponding stage of the
// manager lifecycle. To avoid code churn and implementing unnecessary
// functions, the Modifier interface is split into sub-interfaces, each with a
// single function. This way, the developer can implement only the functions
// they need, and the manager will call them at the right time. Note regarding
// internal state of the modifier: if the modifier is added to the list of
// modifiers enabled by default (see NewManagerWithDefault above), all managers
// with those default modifiers will share the same instance of the modifier. On
// the other hand, if the modifier is added to a specific manager, it can have
// its own instance of the modifier, unless the caller explicitly uses the same
// modifier instance with different managers. In other words, if the modifier is
// to have any internal state specific to each manager, it should not be added
// to the list of default modifiers, and developers using it should be aware of
// this behavior.
type Modifier interface {
	fmt.Stringer
}

// ModifierBeforeInit is a sub-interface of Modifier that exposes a BeforeInit method
type ModifierBeforeInit interface {
	Modifier

	// BeforeInit is called before the ebpf.Manager.InitWithOptions call
	// names.ModuleName refers to the name associated with Manager instance. An
	// error returned from this function will stop the initialization process.
	BeforeInit(*manager.Manager, names.ModuleName, *manager.Options) error
}

// ModifierAfterInit is a sub-interface of Modifier that exposes an AfterInit method
type ModifierAfterInit interface {
	Modifier

	// AfterInit is called after the ebpf.Manager.InitWithOptions call
	AfterInit(*manager.Manager, names.ModuleName, *manager.Options) error
}

// ModifierPreStart is a sub-interface of Modifier that exposes an PreStart method
type ModifierPreStart interface {
	Modifier

	// PreStart is called before the ebpf.Manager.Start call
	PreStart(*manager.Manager, names.ModuleName) error
}

// ModifierBeforeStop is a sub-interface of Modifier that exposes a BeforeStop method
type ModifierBeforeStop interface {
	Modifier

	// BeforeStop is called before the ebpf.Manager.Stop call. An error returned
	// from this function will not prevent the manager from stopping, but it will
	// be logged.
	BeforeStop(*manager.Manager, names.ModuleName, manager.MapCleanupType) error
}

// ModifierAfterStop is a sub-interface of Modifier that exposes an AfterStop method
type ModifierAfterStop interface {
	Modifier

	// AfterStop is called after the ebpf.Manager.Stop call. An error returned
	// from this function will be logged.
	AfterStop(*manager.Manager, names.ModuleName, manager.MapCleanupType) error
}

func runModifiersOfType[K Modifier](modifiers []Modifier, funcName string, runner func(K) error) error {
	var errs error
	for _, mod := range modifiers {
		if as, ok := mod.(K); ok {
			if err := runner(as); err != nil {
				errs = errors.Join(errs, fmt.Errorf("error running %s manager modifier %s: %w", mod, funcName, err))
			}
		}
	}
	return errs
}

// InitWithOptions is a wrapper around ebpf-manager.Manager.InitWithOptions
func (m *Manager) InitWithOptions(bytecode io.ReaderAt, opts *manager.Options) error {
	// we must load the ELF file before initialization,
	// to build the collection specs, because some modifiers
	// inspect these to make changes to the eBPF resources.
	if err := m.LoadELF(bytecode); err != nil {
		return fmt.Errorf("failed to load elf from reader: %w", err)
	}

	err := runModifiersOfType(m.EnabledModifiers, "BeforeInit", func(mod ModifierBeforeInit) error {
		return mod.BeforeInit(m.Manager, m.Name, opts)
	})
	if err != nil {
		return err
	}

	if err := m.Manager.InitWithOptions(nil, *opts); err != nil {
		return err
	}

	return runModifiersOfType(m.EnabledModifiers, "AfterInit", func(mod ModifierAfterInit) error {
		return mod.AfterInit(m.Manager, m.Name, opts)
	})
}

// Stop is a wrapper around ebpf-manager.Manager.Stop
func (m *Manager) Stop(cleanupType manager.MapCleanupType) error {
	var errs error

	err := runModifiersOfType(m.EnabledModifiers, "BeforeStop", func(mod ModifierBeforeStop) error {
		return mod.BeforeStop(m.Manager, m.Name, cleanupType)
	})
	if err != nil {
		errs = errors.Join(errs, err)
	}

	if err := m.Manager.Stop(cleanupType); err != nil {
		errs = errors.Join(errs, fmt.Errorf("failed to stop manager %w", err))
	}

	err = runModifiersOfType(m.EnabledModifiers, "AfterStop", func(mod ModifierAfterStop) error {
		return mod.AfterStop(m.Manager, m.Name, cleanupType)
	})

	return errors.Join(errs, err)
}

// Start is a wrapper around ebpf-manager.Manager.Start
func (m *Manager) Start() error {
	err := runModifiersOfType(m.EnabledModifiers, "PreStart", func(mod ModifierPreStart) error {
		return mod.PreStart(m.Manager, m.Name)
	})
	if err != nil {
		return err
	}

	return m.Manager.Start()
}
