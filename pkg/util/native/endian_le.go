// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build 386 || amd64 || amd64p32 || arm || arm64 || mipsle || mips64le || mips64p32le || ppc64le || riscv64

// Package native provides the endianness of an architecture
package native

import "encoding/binary"

// Endian is set to either binary.BigEndian or binary.LittleEndian,
// depending on the host's endianness.
var Endian binary.ByteOrder = binary.LittleEndian
