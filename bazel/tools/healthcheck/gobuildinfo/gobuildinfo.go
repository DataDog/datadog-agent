// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Command gobuildinfo reports Go executables, read from stdin one path per line, whose build info
// lacks the main module.
package main

import (
	"bufio"
	"debug/buildinfo"
	"fmt"
	"os"
	"strings"
)

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		path := scanner.Text()
		info, err := buildinfo.ReadFile(path)
		if err != nil {
			// debug/buildinfo keeps its errNotGoExe sentinel unexported: https://go.dev/issue/67401
			if strings.HasSuffix(err.Error(), "not a Go executable") {
				continue
			}
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if problem := check(info); problem != "" {
			fmt.Printf("%s\t%s\n", path, problem)
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func check(info *buildinfo.BuildInfo) string {
	switch mainModule := info.Main.Path; {
	case mainModule == "":
		return "no main module"
	case info.Path != mainModule && !strings.HasPrefix(info.Path, mainModule+"/"):
		return fmt.Sprintf("main package %s outside main module %s", info.Path, mainModule)
	}
	return ""
}
