// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package amd contains the AMD GPU core check, which collects the metrics of
// AMD GPUs driven by the amdgpu kernel driver from sysfs.
package amd

// CheckName defines the name of the AMD GPU check
const CheckName = "amd_gpu"
