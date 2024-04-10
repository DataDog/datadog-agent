// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ebpf

import (
	"fmt"
	"io"
	"sync"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Manager wraps ebpf-manager.Manager, adding a property with the list of enabled modifiers
// for this instance.
type Manager struct {
	*manager.Manager
	EnabledModifiers []Modifier // List of enabled modifiers
}

var modifiersSync sync.Once

// defaultModifiers is a list of modifiers that are enabled by default when the manager wrapper is initialized using the relevant ctor (see NewManagerWithDefault).
// This is a static list lazy-initialized once during the lifetime of the program, hence the modifiers in this list must be stateless.
var defaultModifiers []Modifier

// NewManager creates a manager wrapper.
// Optionally one can provide a list of modifiers to attach to the manager
func NewManager(mgr *manager.Manager, modifiers ...Modifier) *Manager {
	log.Tracef("Creating new manager with modifiers: %v", modifiers)
	return &Manager{
		Manager:          mgr,
		EnabledModifiers: modifiers,
	}
}

// NewManagerWithDefault creates a manager wrapper with default modifiers.
func NewManagerWithDefault(mgr *manager.Manager, modifiers ...Modifier) *Manager {
	modifiersSync.Do(func() {
		defaultModifiers = []Modifier{&PrintkPatcherModifier{}}
	})
	return NewManager(mgr, append(defaultModifiers, modifiers...)...)
}

// Modifier is an interface that can be implemented by a package to
// add functionality to the ebpf.Manager. It exposes a name to identify the modifier,
// two functions that will be called before and after the ebpf.Manager.InitWithOptions
// call, and a function that will be called when the manager is stopped.
// Note regarding internal state of the modifier: if the modifier is added to the list of modifiers
// enabled by default (pkg/ebpf/ebpf.go:registerDefaultModifiers), all managers with those default modifiers
// will share the same instance of the modifier. On the other hand, if the modifier is added to a specific
// manager, it can have its own instance of the modifier, unless the caller explicitly uses the same modifier
// instance with different managers. In other words, if the modifier is to have any internal state specific to
// each manager, it should not be added to the list of default modifiers, and developers using it
// should be aware of this behavior.
type Modifier interface {
	fmt.Stringer
	// BeforeInit is called before the ebpf.Manager.InitWithOptions call
	BeforeInit(*manager.Manager, *manager.Options) error

	// AfterInit is called after the ebpf.Manager.InitWithOptions call
	AfterInit(*manager.Manager, *manager.Options) error
}

// InitWithOptions is a wrapper around ebpf-manager.Manager.InitWithOptions
func (m *Manager) InitWithOptions(bytecode io.ReaderAt, opts *manager.Options) error {
	for _, mod := range m.EnabledModifiers {
		log.Tracef("Running %s manager modifier BeforeInit", mod)
		if err := mod.BeforeInit(m.Manager, opts); err != nil {
			return fmt.Errorf("error running %s manager modifier: %w", mod, err)
		}
	}

	if err := m.Manager.InitWithOptions(bytecode, *opts); err != nil {
		return err
	}

	for _, mod := range m.EnabledModifiers {
		log.Tracef("Running %s manager modifier AfterInit", mod)
		if err := mod.AfterInit(m.Manager, opts); err != nil {
			return fmt.Errorf("error running %s manager modifier: %w", mod, err)
		}
	}
	return nil
}
