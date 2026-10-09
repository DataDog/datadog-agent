// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !((linux || (darwin && cgo)) && (amd64 || arm64))

package checksfit

// Supported reports whether this build carries a usable FIT transport. This
// build links the transport stub, so the forwarder must not be installed.
const Supported = false
