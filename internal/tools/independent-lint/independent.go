// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

// Package independent-lint checks a go.mod file at a given path specified by the -path argument
// to ensure that it does not import the list of modules specified by the -deny argument. If
// the module is found, it exits with status code 1 and logs an error.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
)

var (
	path = flag.String("path", ".", "Path to go.mod file to check")
	deny = flag.String("deny", "github.com/DataDog/datadog-agent", "Comma-separated list of imports to deny")
)

func main() {
	flag.Parse()
	if *path == "" || *deny == "" {
		flag.PrintDefaults()
		os.Exit(1)
	}
	denylist := map[string]struct{}{}
	for _, p := range strings.Split(*deny, ",") {
		denylist[strings.TrimSpace(p)] = struct{}{}
	}
	b, err := os.ReadFile(filepath.Join(*path, "go.mod"))
	if err != nil {
		fmt.Println("Error opening file %q:", err)
		return
	}
	f, err := modfile.Parse("go.mod", b, func(_, v string) (string, error) {
		return module.CanonicalVersion(v), nil
	})
	for _, req := range f.Require {
		if len(req.Syntax.Token) < 1 {
			continue
		}
		// From https://go.dev/ref/mod#go-mod-file-require
		// RequireSpec = ModulePath Version newline .
		modname := req.Syntax.Token[0]
		if _, ok := denylist[modname]; ok {
			fmt.Printf("Error: %q is not an allowed import for %q.\n", modname, *path)
			os.Exit(1)
		}
	}
}
