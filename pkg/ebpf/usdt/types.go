// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build ignore

package usdt

/*
#include "../c/usdt_types.h"
*/
import "C"

// Spec is the argument specification of a USDT probe site, as stored in the
// usdt_specs eBPF map of pkg/ebpf/c/usdt.h.
type Spec C.struct_usdt_spec

// ArgSpec is the specification of one argument of a USDT probe site.
type ArgSpec C.struct_usdt_arg_spec

// Values of ArgSpec.Arg_type.
const (
	ArgConst    = C.USDT_ARG_CONST
	ArgReg      = C.USDT_ARG_REG
	ArgRegDeref = C.USDT_ARG_REG_DEREF
)

const (
	// MaxArgs is the maximum number of arguments of a USDT probe site.
	MaxArgs = C.USDT_MAX_ARG_CNT
	// MaxSpecs is the number of entries of the usdt_specs eBPF map.
	MaxSpecs = C.USDT_MAX_SPEC_CNT
)
