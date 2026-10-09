// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build (linux || (darwin && cgo)) && (amd64 || arm64)

package checksfit

// Supported reports whether this build carries a usable FIT transport. The
// transport supports little-endian amd64/arm64 Linux and macOS with cgo
// enabled; other platforms link a stub whose calls fail with
// fitcore.ErrUnsupported, so callers that depend on FIT must check this
// constant before installing a forwarder.
const Supported = true
