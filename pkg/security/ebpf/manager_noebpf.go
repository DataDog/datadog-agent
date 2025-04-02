// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && !linux_bpf

// Package ebpf holds ebpf related files
package ebpf

import (
	manager "github.com/DataDog/ebpf-manager"
)

// Manager is wrapper type for ebpf-manager used when the linux_bpf build tag isn't set
type Manager struct {
	*manager.Manager
}

// Get returns the ebpf-manager instance
func (m *Manager) Get() *manager.Manager {
	return m.Manager
}

// NewDefaultOptions returns a new instance of the default runtime security manager options
func NewDefaultOptions() manager.Options {
	return manager.Options{}
}

// NewRuntimeSecurityManager returns a new instance of the runtime security module manager
func NewRuntimeSecurityManager(_ bool, _ bool) ManagerInterface {
	return &Manager{
		Manager: &manager.Manager{},
	}
}
