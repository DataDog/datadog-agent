// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package redaction decides which captured values the dynamic instrumentation
// snapshot pipeline must scrub before they leave the host.
//
// It mirrors the redaction model used by the Java, .NET, and Python tracers:
// a value is redacted either because the name of the variable, field, or
// string map key that holds it matches a sensitive keyword, or because the
// value's type name matches a redacted type. The default keyword set is the
// shared cross-language list (originally derived from the Sentry SDK
// scrubber). Identifiers are normalized by lowercasing and stripping the
// separators _ - $ @ so that matching is case- and separator-insensitive.
//
// Unlike the in-process tracers, Go's dynamic instrumentation captures values
// out of process in system-probe, so redaction is enforced when the captured
// data is decoded. The decoder consults the Config to drop a captured value
// whose field or variable name, type, or string map key is redacted, emitting
// the redactedIdent / redactedType not-captured reasons. irgen also consults
// it to reject probe conditions that reference a redacted identifier (a
// condition is evaluated in eBPF and would otherwise leak the value through
// the fire/no-fire result) and to mark capture expressions that reference one
// so the decoder drops them.
package redaction
