// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package yara contains an event monitor consumer that scans executed binaries with YARA rules,
// once per distinct file content.
//
// The pipeline is split into independent stages that talk through the contracts in types.go:
//   - the exec consumer builds an ExecFile from each exec event (hot path, no I/O)
//   - a Deduper filters out already-seen file identities and contents
//   - a ScanPool runs a Scanner over the file content on its own workers
//   - a Reporter emits the results
//
// standin.go provides trivial implementations of these contracts, used for tests and for the
// cgo-free dry run.
package yara
