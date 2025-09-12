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

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Short: "PC symbolication utilities",
}

func init() {
	rootCmd.AddCommand(addr2lineCmd)
	rootCmd.AddCommand(listCmd)
}

var addr2lineCmd = &cobra.Command{
	Use:   "addr2line <binary> <pc>",
	Short: "Resolve a PC to function@file:line",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) (retErr error) {
		pc, err := strconv.ParseUint(args[1], 0, 64)
		if err != nil {
			return fmt.Errorf("invalid pc: %w", err)
		}
		binary := args[0]
		cmd.SilenceUsage = true
		symtab, err := object.OpenGoSymbolTable(binary)
		if err != nil {
			return err
		}
		defer func() { retErr = errors.Join(retErr, symtab.Close()) }()
		locations := symtab.LocatePC(pc)
		if len(locations) == 0 {
			return fmt.Errorf("no location found for pc 0x%x", pc)
		}
		for _, location := range locations {
			fmt.Printf("%s@%s:%d\n", location.Function, location.File, location.Line)
		}
		return nil
	},
}

var listCmd = &cobra.Command{
	Use:   "list <binary>",
	Short: "List all functions in a Go binary",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) (retErr error) {
		binary := args[0]
		cmd.SilenceUsage = true

		symtab, err := object.OpenGoSymbolTable(binary)
		if err != nil {
			return err
		}
		defer func() { retErr = errors.Join(retErr, symtab.Close()) }()
		for fn := range symtab.Functions() {
			fmt.Printf("%s %#x-%#x\n", fn.Name(), fn.Entry, fn.End)
		}
		return nil
	},
}
