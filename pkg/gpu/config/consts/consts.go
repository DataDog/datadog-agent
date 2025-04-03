// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package consts provides constants for the GPU monitoring config, so that they can be
// imported by other packages without importing the entire config package, which includes
// ebpf config
package consts

// GPUNS is the namespace for the GPU monitoring probe.
const GPUNS = "gpu_monitoring"
