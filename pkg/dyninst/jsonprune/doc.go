// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package jsonprune trims oversized Dynamic Instrumentation snapshot JSON
// documents to fit within a fixed byte budget before they are shipped to the
// Event Platform.
//
// Pruning is size-driven: if the input is already within budget, the input is
// returned unchanged. Otherwise the document is parsed once to locate every
// captured-value object (at level >= 5 in the snapshot schema), and the
// largest/deepest ones are replaced with a small placeholder until the total
// fits. The algorithm implements the "Dynamic Instrumentation JSON Snapshot
// Pruning Algorithm" RFC.
package jsonprune
