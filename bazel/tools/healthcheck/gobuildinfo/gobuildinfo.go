// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Command gobuildinfo reports Go executables, read from stdin one path per line, whose build info
// lacks a versioned main module or pinned dependencies, first-party ones exempt from pinning.
package main

import (
	"bufio"
	"debug/buildinfo"
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"

	"golang.org/x/mod/semver"
)

func main() {
	metadataPath := flag.String("first-party-metadata", "", "package_metadata JSON of the first-party module")
	flag.Parse()
	firstParty, err := modulePath(*metadataPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
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
		if problems := check(info, firstParty); len(problems) > 0 {
			fmt.Printf("%s\t%s\n", path, strings.Join(problems, ", "))
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// modulePath reads a pkg:golang PURL's module path as rules_go does, ignoring its version.
func modulePath(metadataPath string) (string, error) {
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return "", err
	}
	var metadata struct{ Purl string }
	if err := json.Unmarshal(data, &metadata); err != nil {
		return "", fmt.Errorf("%s: %w", metadataPath, err)
	}
	u, err := url.Parse(metadata.Purl)
	if err != nil {
		return "", fmt.Errorf("%s: %w", metadataPath, err)
	}
	value, ok := strings.CutPrefix(u.Opaque, "golang/")
	if u.Scheme != "pkg" || !ok {
		return "", fmt.Errorf("%s: %q is not a pkg:golang PURL", metadataPath, metadata.Purl)
	}
	path, _, _ := strings.Cut(value, "@")
	return path, nil
}

func check(info *buildinfo.BuildInfo, firstParty string) []string {
	mainModule := info.Main.Path
	if mainModule == "" {
		return []string{"no main module"}
	}
	var problems []string
	if info.Path != mainModule && !strings.HasPrefix(info.Path, mainModule+"/") {
		problems = append(problems, fmt.Sprintf("main package %s outside main module %s", info.Path, mainModule))
	}
	if !semver.IsValid(info.Main.Version) {
		problems = append(problems, fmt.Sprintf("main module %s has invalid version %q", mainModule, info.Main.Version))
	}
	for _, dep := range info.Deps {
		if dep.Replace != nil {
			dep = dep.Replace
		}
		pinned := strings.HasPrefix(dep.Sum, "h1:") || dep.Path == firstParty || strings.HasPrefix(dep.Path, firstParty+"/")
		if !semver.IsValid(dep.Version) || !pinned {
			problems = append(problems, fmt.Sprintf("dependency %s@%s lacks a version or checksum", dep.Path, dep.Version))
		}
	}
	return problems
}
