// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package main checks that minimized kernel BTFs relocate CO-RE programs the
// way the full kernel BTFs do, with cilium/ebpf, which the agent uses at load
// time.
package main

import (
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/btf"
)

type program struct {
	name  string
	spec  *ebpf.CollectionSpec
	relos []*btf.CORERelocation
}

func main() {
	if len(os.Args) < 4 {
		fmt.Fprintln(os.Stderr, "usage: corecheck FULL_DIR MIN_DIR OBJECT...")
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2], os.Args[3:]); err != nil {
		fmt.Fprintf(os.Stderr, "corecheck: %v\n", err)
		os.Exit(1)
	}
}

func run(fullDir, minDir string, objects []string) error {
	progs, err := loadPrograms(objects)
	if err != nil {
		return err
	}
	var kernels, mismatches int
	err = filepath.WalkDir(minDir, func(minPath string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(minDir, minPath)
		if err != nil {
			return err
		}
		n, err := checkKernel(filepath.Join(fullDir, rel), minPath, progs)
		if err != nil {
			return fmt.Errorf("%s: %w", rel, err)
		}
		kernels++
		mismatches += n
		return nil
	})
	if err != nil {
		return err
	}
	if kernels == 0 {
		return fmt.Errorf("no minimized BTFs in %s", minDir)
	}
	if mismatches > 0 {
		return fmt.Errorf("%d program relocations differ between minimized and full BTFs", mismatches)
	}
	fmt.Printf("%d programs relocate identically against %d minimized BTFs\n", len(progs), kernels)
	return nil
}

// loadPrograms returns the programs of objects that have CO-RE relocations.
// cilium/ebpf relocates each program on its own, with its subprograms.
func loadPrograms(objects []string) ([]program, error) {
	var progs []program
	for _, path := range objects {
		spec, err := ebpf.LoadCollectionSpec(path)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		for _, name := range slices.Sorted(maps.Keys(spec.Programs)) {
			insns := spec.Programs[name].Instructions
			var relos []*btf.CORERelocation
			for i := range insns {
				if r := btf.CORERelocationMetadata(&insns[i]); r != nil {
					relos = append(relos, r)
				}
			}
			if len(relos) > 0 {
				progs = append(progs, program{name: filepath.Base(path) + "/" + name, spec: spec, relos: relos})
			}
		}
	}
	return progs, nil
}

// checkKernel returns how many relocations differ against the minimized BTF at
// minPath from the full BTF at fullPath. A program that fails to relocate
// counts once.
func checkKernel(fullPath, minPath string, progs []program) (int, error) {
	full, err := btf.LoadSpec(fullPath)
	if err != nil {
		return 0, err
	}
	minimized, err := btf.LoadSpec(minPath)
	if err != nil {
		return 0, err
	}
	var mismatches int
	report := func(p program, format string, args ...any) {
		fmt.Fprintf(os.Stderr, "%s on %s: %s\n", p.name, minPath, fmt.Sprintf(format, args...))
		mismatches++
	}
	for _, p := range progs {
		// An error fails all relocations of the program, which would hide the others.
		// It fails the check even against the full BTF: the agent could not load
		// the program on that kernel either.
		want, err := relocate(p, full)
		if err != nil {
			report(p, "full BTF: %v", err)
			continue
		}
		got, err := relocate(p, minimized)
		if err != nil {
			report(p, "minimized BTF: %v", err)
			continue
		}
		for i := range want {
			if !sameFixup(want[i].String(), got[i].String(), full, minimized) {
				report(p, "%s: full BTF %s, minimized BTF %s", p.relos[i], want[i].String(), got[i].String())
			}
		}
	}
	return mismatches, nil
}

func relocate(p program, kernel *btf.Spec) ([]btf.COREFixup, error) {
	return btf.CORERelocate(p.relos, []*btf.Spec{kernel}, p.spec.ByteOrder, p.spec.Types.TypeID)
}

// sameFixup compares fixups by their String(), "kind=local->target" or
// "kind=poison". Minimizing renumbers kernel types, so target_type_id fixups
// match when their targets have the same kind and name.
func sameFixup(want, got string, full, minimized *btf.Spec) bool {
	const typeID = "target_type_id="
	wantLocal, wantTarget, ok1 := strings.Cut(strings.TrimPrefix(want, typeID), "->")
	gotLocal, gotTarget, ok2 := strings.Cut(strings.TrimPrefix(got, typeID), "->")
	if !strings.HasPrefix(want, typeID) || !strings.HasPrefix(got, typeID) || !ok1 || !ok2 {
		return want == got
	}
	if wantLocal != gotLocal {
		return false
	}
	wantType, err := targetType(wantTarget, full)
	if err != nil {
		return false
	}
	gotType, err := targetType(gotTarget, minimized)
	if err != nil {
		return false
	}
	return fmt.Sprintf("%T %s", wantType, wantType.TypeName()) == fmt.Sprintf("%T %s", gotType, gotType.TypeName())
}

func targetType(id string, spec *btf.Spec) (btf.Type, error) {
	n, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		return nil, err
	}
	return spec.TypeByID(btf.TypeID(n))
}
