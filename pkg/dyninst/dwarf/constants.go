// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

// Package dwarf contains DWARF-related functionality.
package dwarf

// DW_INL_inlined is the value for AttrInline marking the function as inlined.
const DW_INL_inlined = 1 //nolint:revive

// DW_LANG_Go is the constant for the Go language in AttrLanguage fields.
const DW_LANG_Go = 0x0016 //nolint:revive
