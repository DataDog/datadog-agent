// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package usdt reads USDT (SystemTap SDT) probes from ELF files and encodes
// their argument specifications for the eBPF helpers of pkg/ebpf/c/usdt.h.
// It is only implemented on Linux.
//
// A USDT probe site is a nop instruction described by a note of the
// .note.stapsdt section. The arguments of a site are described by a string of
// assembler operands, e.g. "-4@-20(%rbp) 8@%rax" on x86-64 or
// "8@[sp, 112] 8@x1" on arm64.
//
// Typical use:
//
//   - ReadProbes lists the probe sites of a file, with the file offset to
//     attach a uprobe to and the parsed argument Spec. Sites that cannot be
//     attached safely have a non-nil Probe.Err and must be skipped. Only
//     attach uprobes to files targeting the running architecture (see
//     HostMachine): the offsets and the specs are meaningless elsewhere.
//   - SpecRegistry deduplicates the Specs of the attached sites and assigns
//     each distinct one an id in [1, MaxSpecs). The caller stores the Spec
//     under its id in the SpecsMapName eBPF map, and attaches every site
//     using it with a program instance whose SpecIDConstant constant is that
//     id.
//   - In eBPF, usdt_arg(ctx, n, &val) decodes the n-th argument of the site
//     that triggered the program.
//
// Spec and ArgSpec are generated from pkg/ebpf/c/usdt_types.h (see types.go):
// Spec.Cnt is the number of arguments, described by Spec.Args[:Spec.Cnt].
package usdt
