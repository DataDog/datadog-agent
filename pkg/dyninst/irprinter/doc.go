// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package irprinter provides tooling to print the IR in various formats.
//
// This package is intended for use with testing, and should not be considered
// to be part of any public API or to convey any stability guarantees. The
// lack of deserializing also limits the scope of the package.
//
// This is in separate package from in part because of the desire to avoid a
// stable API as described above, and also because these types form a graph that
// may be cyclic and thus not trivially serializable.
package irprinter
