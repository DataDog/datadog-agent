// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ebpf

import (
	"fmt"
	"io"
	"reflect"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Manager wraps ebpf-manager.Manager, adding a property with the list of enabled modifiers
// for this instance.
type Manager struct {
	*manager.Manager
	EnabledModifiers []Modifier // List of enabled modifiers
}

// defaultModifiers is a list of modifiers that are enabled by default when the callers don't provide
// a specific list. This list is filled by the pkg/ebpf/ebpf.go:registerDefaultModifiers function.
var defaultModifiers []Modifier

// NewManager creates a manager wrapper.
// If the modifiers list is empty, it will be initialized with the default modifiers.
// Pass nil as the argument (example: mgr, err := NewManager(mgr, nil)) to disable all modifiers.
func NewManager(mgr *manager.Manager, modifiers ...Modifier) *Manager {
	if len(modifiers) == 0 {
		modifiers = defaultModifiers
	} else if len(modifiers) == 1 && modifiers[0] == nil {
		modifiers = nil
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
	// BeforeInit is called before the ebpf.Manager.InitWithOptions call
	BeforeInit(*Manager, *manager.Options) error

	// AfterInit is called after the ebpf.Manager.InitWithOptions call
	AfterInit(*Manager, *manager.Options) error
}

// InitWithOptions is a wrapper around ebpf-manager.Manager.InitWithOptions
func (m *Manager) InitWithOptions(bytecode io.ReaderAt, opts *manager.Options) error {
	for _, mod := range m.EnabledModifiers {
		modName := reflect.TypeOf(mod).String()
		log.Debugf("Running %s manager modifier", modName)
		if err := mod.BeforeInit(m, opts); err != nil {
			return fmt.Errorf("error running %s manager modifier: %w", modName, err)
		}
	}

	if err := m.Manager.InitWithOptions(bytecode, *opts); err != nil {
		return err
	}

	for _, mod := range m.EnabledModifiers {
		modName := reflect.TypeOf(mod).String()
		log.Debugf("Running %s manager modifier", modName)
		if err := mod.AfterInit(m, opts); err != nil {
			return fmt.Errorf("error running %s manager modifier: %w", modName, err)
		}
	}
	return nil
}
