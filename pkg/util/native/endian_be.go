// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build armbe || arm64be || mips || mips64 || mips64p32 || ppc64 || s390 || s390x || sparc || sparc64

package native

import "encoding/binary"

// Endian is set to either binary.BigEndian or binary.LittleEndian,
// depending on the host's endianness.
var Endian binary.ByteOrder = binary.BigEndian
