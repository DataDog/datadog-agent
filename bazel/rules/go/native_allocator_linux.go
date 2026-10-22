// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && cgo

package main

// This source belongs to the final binary, never to rtloader or a shared library.
// rules_go only processes cdeps' linker options when it compiles Cgo sources.

// #include <stdlib.h>
import "C"
