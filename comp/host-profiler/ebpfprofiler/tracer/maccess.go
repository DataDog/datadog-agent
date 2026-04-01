// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package tracer

import (
	"errors"
	"fmt"

	cebpf "github.com/cilium/ebpf"
	"go.opentelemetry.io/ebpf-profiler/kallsyms"
	"go.opentelemetry.io/ebpf-profiler/libpf"
	"go.opentelemetry.io/ebpf-profiler/maccess"
)

// checkForMmaccessPatch validates if a Linux kernel function is patched by
// extracting the kernel code of the function and analyzing it.
func checkForMaccessPatch(coll *cebpf.CollectionSpec, maps map[string]*cebpf.Map,
	kmod *kallsyms.Module) error {
	faultyFunc, err := kmod.LookupSymbol("copy_from_user_nofault")
	if err != nil {
		return fmt.Errorf("failed to lookup 'copy_from_user_nofault': %v", err)
	}
	code, err := loadKernelCode(coll, maps, libpf.SymbolValue(faultyFunc))
	if err != nil {
		return fmt.Errorf("failed to load kernel code for 'copy_from_user_nofault': %v", err)
	}

	newCheckFunc, _ := kmod.LookupSymbol("nmi_uaccess_okay")
	patched, err := maccess.CopyFromUserNoFaultIsPatched(code,
		uint64(faultyFunc), uint64(newCheckFunc))
	if err != nil {
		return fmt.Errorf("failed to check if 'copy_from_user_nofault' is patched: %v", err)
	}
	if !patched {
		return errors.New("kernel is not patched")
	}
	return nil
}
