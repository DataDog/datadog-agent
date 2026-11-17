// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows && (386 || arm)

package procinfo

// sizeofProcessEntry32 is the byte size of windows.ProcessEntry32 on 32-bit
// Windows. Manually verified from the struct layout:
//
//	uint32×3 + uintptr(4) + uint32×5 + [260]uint16 = 36 + 520 = 556
const sizeofProcessEntry32 = uint32(556)
