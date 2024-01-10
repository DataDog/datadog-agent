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
	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
	"log"
	"os"
	"path/filepath"
	"strings"
)

var (
	path       = flag.String("path", ".", "Path to go.mod file to format")
	formatFile = flag.Bool("formatFile", false, "Enable or Disable formatting of the mod file")
)

func main() {
	flag.Parse()
	if *path == "" {
		flag.PrintDefaults()
		os.Exit(1)
	}
	modFilePath := filepath.Join(*path, "go.mod")
	b, err := os.ReadFile(modFilePath)
	if err != nil {
		fmt.Println("Error opening file %q:", err)
		return
	}
	f, err := modfile.Parse("go.mod", b, func(_, v string) (string, error) {
		return module.CanonicalVersion(v), nil
	})

	ReplaceMap := make(map[string]bool)
	for i := 0; i < len(f.Replace); i += 1 {
		token := f.Replace[i].Syntax.Token
		if len(token) < 1 {
			continue
		}
		ReplaceMap[token[0]] = true
	}
	writeNeeded := false
	for _, req := range f.Require {
		if len(req.Syntax.Token) < 1 {
			continue
		}

		modname := req.Syntax.Token[0]
		if !strings.HasPrefix(modname, "github.com/DataDog/datadog-agent") {
			continue
		}

		if !ReplaceMap[modname] {
			writeNeeded = true
			modversion := req.Syntax.Token[1]

			fmt.Printf("Required %s %s is missing in replace of %s\n", modname, modversion, modFilePath)
			if *formatFile {
				trimName := strings.Split(modname, "github.com/DataDog/datadog-agent/")[1]
				trimPath := strings.Split(*path, "github.com/DataDog/datadog-agent/")[1]
				relativePath, err := filepath.Rel(trimPath, trimName)
				if err != nil {
					log.Fatal(err)
				}
				err = f.AddReplace(modname, modversion, relativePath, modversion)
				if err != nil {
					log.Fatal(err)
				}
				fmt.Printf("%s has been formatted !\n", modFilePath)
			}
		}
	}
	if writeNeeded {
		data, err := f.Format()
		if err != nil {
			log.Fatal(err)
		}
		err = os.WriteFile(modFilePath, data, 0644)
		if err != nil {
			log.Fatal(err)
		}
	}

}
