// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package main generates pkg/security/utils/syscalls.go — a combined amd64+arm64
// SyscallKey map — from the upstream Linux kernel syscall tables.
package main

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"go/format"
	"io"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"text/template"
)

func main() {
	var (
		amd64TablePath string
		arm64TablePath string
		outputPath     string
	)

	flag.StringVar(&amd64TablePath, "amd64-table", "", "Path to the x86_64 .tbl syscall table")
	flag.StringVar(&arm64TablePath, "arm64-table", "", "Path to the arm64 unistd.h syscall table")
	flag.StringVar(&outputPath, "output", "", "Output path of the generated Go file")
	flag.Parse()

	if amd64TablePath == "" || arm64TablePath == "" || outputPath == "" {
		fmt.Fprintf(os.Stderr, "Please provide all required flags\n")
		flag.Usage()
		os.Exit(1)
	}

	amd64Syscalls, err := parseFile(amd64TablePath, parseSyscallTableLine([]string{"common", "64"}))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse amd64 table: %v\n", err)
		os.Exit(1)
	}

	arm64Syscalls, err := parseFile(arm64TablePath, parseUnistdTableLine)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse arm64 table: %v\n", err)
		os.Exit(1)
	}

	content, err := generateMapCode(amd64Syscalls, arm64Syscalls)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to generate code: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(outputPath, content, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write output: %v\n", err)
		os.Exit(1)
	}
}

type syscallEntry struct {
	Arch   string
	Number int
	Name   string
}

func parseFile(path string, perLine func(string) (*syscallEntry, error)) ([]*syscallEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parseReader(f, perLine)
}

func parseReader(r io.Reader, perLine func(string) (*syscallEntry, error)) ([]*syscallEntry, error) {
	scanner := bufio.NewScanner(r)
	var syscalls []*syscallEntry

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		entry, err := perLine(line)
		if err != nil {
			return nil, err
		}
		if entry != nil {
			syscalls = append(syscalls, entry)
		}
	}

	return syscalls, scanner.Err()
}

func parseSyscallTableLine(abis []string) func(string) (*syscallEntry, error) {
	return func(line string) (*syscallEntry, error) {
		if line == "" || strings.HasPrefix(line, "#") {
			return nil, nil
		}

		parts := strings.Fields(line)
		if len(parts) < 3 {
			return nil, errors.New("found syscall with missing fields")
		}

		number, err := strconv.ParseInt(parts[0], 10, 0)
		if err != nil {
			return nil, err
		}
		abi := parts[1]
		name := parts[2]

		if slices.Contains(abis, abi) {
			return &syscallEntry{Arch: "amd64", Number: int(number), Name: name}, nil
		}
		return nil, nil
	}
}

var unistdDefinedRe = regexp.MustCompile(`#define __NR(3264)?_([0-9a-zA-Z_]+)\s+([0-9]+)`)

func parseUnistdTableLine(line string) (*syscallEntry, error) {
	subs := unistdDefinedRe.FindStringSubmatch(line)
	if subs == nil {
		return nil, nil
	}
	nr, err := strconv.ParseInt(subs[3], 10, 0)
	if err != nil {
		return nil, err
	}
	return &syscallEntry{Arch: "arm64", Number: int(nr), Name: subs[2]}, nil
}

const outputTemplate = `// Code generated - DO NOT EDIT.
// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package utils groups multiple utils function that can be used by the secl package
package utils

// SyscallKey key representing the arch and syscall id
type SyscallKey struct {
	Arch string
	ID   int
}

// Syscalls maps the (arch,syscall_id) to the syscall string
var Syscalls = map[SyscallKey]string{
	// amd64 syscalls
{{- range .Amd64}}
	{"amd64", {{.Number}}}: "{{.Name}}",
{{- end}}

	// arm64 syscalls
{{- range .Arm64}}
	{"arm64", {{.Number}}}: "{{.Name}}",
{{- end}}
}
`

// deduplicateByID resolves duplicate IDs by keeping the entry with the shorter name.
func deduplicateByID(entries []*syscallEntry) []*syscallEntry {
	seen := make(map[int]*syscallEntry)
	for _, e := range entries {
		if existing, ok := seen[e.Number]; !ok || len(e.Name) < len(existing.Name) {
			seen[e.Number] = e
		}
	}
	result := make([]*syscallEntry, 0, len(seen))
	for _, e := range entries {
		if seen[e.Number] == e {
			result = append(result, e)
		}
	}
	return result
}

type templateData struct {
	Amd64 []*syscallEntry
	Arm64 []*syscallEntry
}

func generateMapCode(amd64, arm64 []*syscallEntry) ([]byte, error) {
	tmpl, err := template.New("utils-syscalls").Parse(outputTemplate)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, templateData{
		Amd64: deduplicateByID(amd64),
		Arm64: deduplicateByID(arm64),
	}); err != nil {
		return nil, err
	}

	return format.Source(buf.Bytes())
}
