// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

// Package nvml wraps access to the NVML library for GPU monitoring.
package nvml

// Library is an interface to the NVML library, with the methods that we need for agent
// use.
type Library interface {
	Init() error
	GetGpuDevices() ([]GpuDevice, error)
	Shutdown() error
}

// GpuDevice represents a GPU device
type GpuDevice interface {
	GetNumMultiprocessors() (int, error)
	GetMaxThreads() (int, error)
}
