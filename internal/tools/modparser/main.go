// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"golang.org/x/mod/modfile"
)

func parseMod(module string) (*modfile.File, error) {
	if !strings.HasSuffix(module, "/") {
		module += "/"
	}

	modFilename := module + "go.mod"

	data, err := os.ReadFile(modFilename)
	if err != nil {
		return nil, fmt.Errorf("could not read go.mod file in %s", module)
	}

	parsedFile, err := modfile.Parse(modFilename, data, nil)
	if err != nil {
		return nil, fmt.Errorf("could not parse go.mod file in %s", module)
	}

	return parsedFile, nil
}

func filter(file *modfile.File, filter string) []string {
	var matches []string
	for _, req := range file.Require {
		for _, token := range req.Syntax.Token {
			if strings.HasPrefix(token, filter) {
				matches = append(matches, token)
			}
		}
	}
	return matches
}

func main() {
	var modPath string
	var prefix string

	flag.StringVar(&modPath, "path", "", "Path to the go module to inspect")
	flag.StringVar(&prefix, "prefix", "", "Prefix used to filter requires")

	flag.Parse()

	// Check that both flags have been set
	if flag.NFlag() != 2 {
		flag.Usage()
		os.Exit(1)
	}

	parsedFile, err := parseMod(modPath)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	for _, match := range filter(parsedFile, prefix) {
		fmt.Println(match)
	}
}
