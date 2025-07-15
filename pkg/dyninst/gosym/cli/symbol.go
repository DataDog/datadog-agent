// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Cli for pc symbolication
package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/dyninst/gosym"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
)

func run(binary string, pc uint64) error {
	mef, err := object.OpenMMappingElfFile(binary)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, mef.Close()) }()
	moduledata, err := object.ParseModuleData(mef)
	if err != nil {
		return err
	}
	goDebugSections, err := moduledata.GoDebugSections(mef)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, goDebugSections.Close()) }()
	goVersion, err := object.ParseGoVersion(mef)
	if err != nil {
		return err
	}
	symtab, err := gosym.ParseGoSymbolTable(
		goDebugSections.PcLnTab.Data,
		goDebugSections.GoFunc.Data,
		moduledata.Text,
		moduledata.EText,
		moduledata.MinPC,
		moduledata.MaxPC,
		goVersion,
	)
	if err != nil {
		return err
	}
	locations := symtab.LocatePC(pc)
	if len(locations) == 0 {
		return fmt.Errorf("no location found for pc 0x%x", pc)
	}
	for _, location := range locations {
		fmt.Printf("%s@%s:%d\n", location.Function, location.File, location.Line)
	}
	return nil
}

func main() {
	if len(os.Args) != 3 {
		fmt.Println("Usage: symbol <binary> <pc>")
		os.Exit(1)
	}
	pc, err := strconv.ParseUint(os.Args[2], 16, 64)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	err = run(os.Args[1], pc)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
