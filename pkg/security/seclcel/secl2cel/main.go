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

	"github.com/DataDog/datadog-agent/pkg/security/seclcel"
)

func main() {
	untyped := flag.Bool("untyped", false,
		"translate without consulting the field types, leaving comparisons against array fields literal")

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), `usage: secl2cel [flags] [expression...]

With no expression, print the SECL fields as CEL types.
With expressions, print the CEL each one translates to and type-check it.

  secl2cel
  secl2cel 'process.ancestors.file.name == "sh"'

flags:
`)
		flag.PrintDefaults()
	}
	flag.Parse()

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
		if _, err := seclcel.CompileWithTypes(env, expr, fieldTypes); err != nil {
			fmt.Fprintf(os.Stderr, "  does not type-check: %v\n", err)
			failed = true
		}
	}

	if failed {
		os.Exit(1)
	}
}
