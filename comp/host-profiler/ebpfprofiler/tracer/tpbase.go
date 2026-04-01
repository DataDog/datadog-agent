// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package tracer

import (
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/comp/host-profiler/ebpfprofiler/internal/log"
	cebpf "github.com/cilium/ebpf"

	"go.opentelemetry.io/ebpf-profiler/kallsyms"
	"go.opentelemetry.io/ebpf-profiler/libc"
	"go.opentelemetry.io/ebpf-profiler/libpf"
)

// This file contains code to extract the offset of the thread pointer base variable in
// the `task_struct` kernel struct, which is needed by e.g. Python and Perl tracers.
// This offset varies depending on kernel configuration, so we have to learn it dynamically
// at run time.
//
// Unfortunately, /dev/kmem is often disabled for security reasons, so a BPF helper is used to
// read the kernel memory in portable manner. This code is then analyzed to get the data.
//
// If you're wondering how to check the disassembly of a kernel function:
// 1) Extract your vmlinuz image (the extract-vmlinux script is in the Linux kernel source tree)
//    linux/scripts/extract-vmlinux /boot/vmlinuz-5.6.11 > kernel.elf
// 2) Find the address of aout_dump_debugregs in the ELF
//    address=$(cat /boot/System.map-5.6.11 | grep "T aout_dump_debugregs" | awk '{print $1}')
// 3) Disassemble the kernel ELF starting at that address:
//    objdump -S --start-address=0x$address kernel.elf | head -20

// loadTPBaseOffset extracts the offset of the thread pointer base variable in the `task_struct`
// kernel struct. This offset varies depending on kernel configuration, so we have to learn
// it dynamically at runtime.
func loadTPBaseOffset(coll *cebpf.CollectionSpec, maps map[string]*cebpf.Map,
	kmod *kallsyms.Module,
) (uint64, error) {
	var tpbaseOffset uint32
	analyzers, err := libc.GetTpBaseAnalyzers()
	if err != nil {
		return 0, err
	}
	for _, analyzer := range analyzers {
		sym, err := kmod.LookupSymbol(analyzer.FunctionName)
		if err != nil {
			continue
		}

		code, err := loadKernelCode(coll, maps, libpf.SymbolValue(sym))
		if err != nil {
			return 0, err
		}

		tpbaseOffset, err = analyzer.Analyze(code)
		if err != nil {
			return 0, fmt.Errorf("%w: %s", err, hex.Dump(code))
		}
		log.Debugf("Found tpbase offset: %v (via %s)", tpbaseOffset, analyzer.FunctionName)
		break
	}

	if tpbaseOffset == 0 {
		return 0, errors.New("no supported symbol found")
	}

	// Sanity-check against reasonable values. We expect something in the ~2000-10000 range,
	// but allow for some additional slack on top of that.
	if tpbaseOffset < 500 || tpbaseOffset > 20000 {
		return 0, fmt.Errorf("tpbase offset %v doesn't look sane", tpbaseOffset)
	}

	return uint64(tpbaseOffset), nil
}
