// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package ebpf holds ebpf related files
package ebpf

import (
	"io"

	manager "github.com/DataDog/ebpf-manager"
)

// ManagerInterface is a wrapper type for ebpf-manager and pkg/ebpf/manager.Manager types
type ManagerInterface interface {
	Get() *manager.Manager
	InitWithOptions(bytecode io.ReaderAt, opts manager.Options) error
	Stop(cleanupType manager.MapCleanupType) error
	Start() error
}
