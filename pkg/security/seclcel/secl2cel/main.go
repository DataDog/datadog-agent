// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package main provides secl2cel, a tool to inspect the CEL environment SECL
// rules translate into.
//
// With no argument it prints the fields the environment declares, as CEL types.
// Given SECL expressions it prints what each one translates to and type-checks
// the result, which is how a rule can be checked against the real field types
// before it is deployed.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/google/cel-go/cel"

	"github.com/DataDog/datadog-agent/pkg/security/seclcel"
)

func main() {
	untyped := flag.Bool("untyped", false,
		"translate without consulting the field types, leaving comparisons against array fields literal")
	policies := flag.String("policies", "",
		"measure coverage over a directory of agent rules and macros instead, evaluating each rule's own test cases through both engines")
	verbose := flag.Bool("verbose", false, "list every case that could not be compared, not only the disagreements")

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), `usage: secl2cel [flags] [expression...]

With no expression, print the SECL fields as CEL types.
With expressions, print the CEL each one translates to and type-check it.

  secl2cel
  secl2cel 'process.ancestors.file.name == "sh"'
  secl2cel -policies ~/dd/security-monitoring/workload-security/agent-rules/linux

flags:
`)
		flag.PrintDefaults()
	}
	flag.Parse()

	if *policies != "" {
		if err := runCoverage(*policies, *verbose); err != nil {
			fmt.Fprintln(os.Stderr, "secl2cel:", err)
			os.Exit(1)
		}
		return
	}

	env, err := seclcel.NewModelEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, "secl2cel:", err)
		os.Exit(1)
	}

	if flag.NArg() == 0 {
		fmt.Print(seclcel.DescribeEnv(env))
		return
	}

	var fieldTypes seclcel.FieldTypes = seclcel.ModelFieldTypes{}
	if *untyped {
		fieldTypes = nil
	}

	failed := false
	for _, expr := range flag.Args() {
		translated, err := seclcel.TranslateWithTypes(expr, fieldTypes)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n  cannot be translated: %v\n", expr, err)
			failed = true
			continue
		}

		fmt.Println(translated)

		// The translation is valid CEL either way; checking it is what catches a
		// field that does not exist or a comparison against the wrong type.
		checked, err := seclcel.CompileWithTypes(env, expr, fieldTypes)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  does not type-check: %v\n", err)
			failed = true
			continue
		}

		// And what actually runs is the optimized form, where each field has become
		// the index it is read by. It is the only way to eyeball that mapping.
		optimized, fields, err := seclcel.Optimize(env, checked)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  cannot be optimized: %v\n", err)
			failed = true
			continue
		}
		source, err := cel.AstToString(optimized)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  cannot be printed: %v\n", err)
			failed = true
			continue
		}
		fmt.Printf("  %s\n  reads %s\n", source, strings.Join(fields, ", "))
	}

	if failed {
		os.Exit(1)
	}
}

// runCoverage measures a rule set and prints the report — see coverage.go.
func runCoverage(dir string, verbose bool) error {
	set, err := readPolicies(dir)
	if err != nil {
		return err
	}
	if len(set.rules) == 0 {
		return fmt.Errorf("no agent rules under %s", dir)
	}

	results, macroFailures, err := measure(set)
	if err != nil {
		return err
	}

	if !reportCoverage(set, results, macroFailures, verbose) {
		os.Exit(1)
	}
	return nil
}
