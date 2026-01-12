// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package main contains a CLI for inspecting Go type metadata.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"slices"

	"github.com/DataDog/datadog-agent/pkg/dyninst/gotype"
	"github.com/DataDog/datadog-agent/pkg/dyninst/gotype/gotypeprinter"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
)

func main() {
	typelinks := flag.Bool("typelinks", false, "just print the typelinks")
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	if err := run(flag.Arg(0), *typelinks); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(binaryPath string, typelinks bool) (err error) {
	mef, err := object.OpenMMappingElfFile(binaryPath)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, mef.Close()) }()

	table, err := gotype.NewTable(mef)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, table.Close()) }()

	tl := mef.Section(".typelink")
	if tl == nil {
		return errors.New("no .typelink section")
	}
	tlMap, err := mef.SectionData(tl)
	if err != nil {
		return fmt.Errorf("failed to mmap .typelink: %w", err)
	}
	defer func() { err = errors.Join(err, tlMap.Close()) }()

	tlData := tlMap.Data()

	// Walk all reachable types and sort them for stable output.
	ids := gotype.ParseTypeLinks(tlData)
	var all []gotype.Type
	if typelinks {
		for id := range ids.TypeIDs() {
			t, err := table.ParseGoType(id)
			if err != nil {
				return err
			}
			all = append(all, t)
		}
	} else {
		all, err = gotypeprinter.WalkTypes(table, slices.Collect(ids.TypeIDs()))
		if err != nil {
			return err
		}
	}

	data, marshalErr := gotypeprinter.TypesToYAML(table, all)
	if marshalErr != nil {
		return marshalErr
	}
	if _, werr := os.Stdout.Write(data); werr != nil {
		return werr
	}
	return nil
}
