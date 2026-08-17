// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !docker && (!cri || !containerd) && !test

package configfilesdiscoveryimpl

// processFallbackRegistry is empty when the build has no container reader.
// Keeping the no-op boundary here avoids linking process-event handling into
// binaries where a process-triggered recollection could never read a config.
type processFallbackRegistry struct{}

func (*component) startProcessFallbackListener() {}

func (*component) stopProcessFallbackListener() {}

func (*adScheduler) registerProcessFallbackLocked(*watchedConfig) {}

func (*adScheduler) removeProcessFallbackLocked(*watchedConfig) {}
