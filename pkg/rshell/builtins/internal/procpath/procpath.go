// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package procpath provides the single canonical default path to the Linux
// proc filesystem. All builtins that read /proc/* reference this constant so
// that the value is defined exactly once.
package procpath

// Default is the default path to the Linux proc filesystem.
const Default = "/proc"
