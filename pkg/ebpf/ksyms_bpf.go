// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ebpf

import (
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
)

type ksymIterProgram struct {
	BpfIteratorDumpKsyms *ebpf.Program `ebpf:"bpf_iter__dump_ksyms"`
}

// GetKernelSymbolsAddressesWithKallsymsIterator will use a bpf iterator program to lookup kernel symbols
// This is useful when due to some lockdown /proc/kallsyms will not return the addresses of kernel symbols
// An example of this is when we are running in a container without CAP_SYSLOG which is needed to ready /proc/kallsyms
func GetKernelSymbolsAddressesWithKallsymsIterator(kernelAddresses ...string) (map[string]uint64, error) {
	var prog ksymIterProgram

	if err := LoadCOREAsset("ksyms_iter.o", func(bc bytecode.AssetReader, managerOptions manager.Options) error {
		collectionSpec, err := ebpf.LoadCollectionSpecFromReader(bc)
		if err != nil {
			return fmt.Errorf("failed to load collection spec: %w", err)
		}

		opts := ebpf.CollectionOptions{
			Programs: ebpf.ProgramOptions{
				LogLevel:    ebpf.LogLevelBranch,
				KernelTypes: managerOptions.VerifierOptions.Programs.KernelTypes,
			},
		}

		if err := collectionSpec.LoadAndAssign(&prog, &opts); err != nil {
			var ve *ebpf.VerifierError
			if errors.As(err, &ve) {
				return fmt.Errorf("verfier error loading collection: %s\n%+v", err, ve)
			}
			return fmt.Errorf("failed to load objects: %w", err)
		}

		return nil
	}); err != nil {
		return nil, err
	}

	iter, err := link.AttachIter(link.IterOptions{
		Program: prog.BpfIteratorDumpKsyms,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to attach bpf iterator: %w", err)
	}
	defer iter.Close()

	ksymsReader, err := iter.Open()
	if err != nil {
		return nil, err
	}
	defer ksymsReader.Close()

	return GetKernelSymbolsAddressesNoCache(ksymsReader, kernelAddresses...)
}
