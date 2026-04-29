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
	"net/http"
	"os"
	"os/exec"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"text/template"
)

func main() {
	var (
		amd64TableURL string
		arm64TableURL string
		outputPath    string
	)

	flag.StringVar(&amd64TableURL, "amd64-table-url", "", "URL of the x86_64 .tbl syscall table")
	flag.StringVar(&arm64TableURL, "arm64-table-url", "", "URL of the arm64 unistd.h syscall table")
	flag.StringVar(&outputPath, "output", "", "Output path of the generated Go file")
	flag.Parse()

	if amd64TableURL == "" || arm64TableURL == "" || outputPath == "" {
		fmt.Fprintf(os.Stderr, "Please provide all required flags\n")
		flag.Usage()
		os.Exit(1)
	}

	amd64Syscalls, err := parseSyscallTable(amd64TableURL, []string{"common", "64"})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse amd64 table: %v\n", err)
		os.Exit(1)
	}

	arm64Syscalls, err := parseUnistdTable(arm64TableURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse arm64 table: %v\n", err)
		os.Exit(1)
	}

	content, err := generateMapCode(amd64Syscalls, arm64Syscalls)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to generate code: %v\n", err)
		os.Exit(1)
	}

	if err := writeFileAndFormat(outputPath, content); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write output: %v\n", err)
		os.Exit(1)
	}
}

type syscallEntry struct {
	Arch   string
	Number int
	Name   string
}

func parseLinuxFile(url string, perLine func(string) (*syscallEntry, error)) ([]*syscallEntry, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
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

func parseSyscallTable(url string, abis []string) ([]*syscallEntry, error) {
	return parseLinuxFile(url, func(line string) (*syscallEntry, error) {
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
	})
}

var unistdDefinedRe = regexp.MustCompile(`#define __NR(3264)?_([0-9a-zA-Z_]+)\s+([0-9]+)`)

func parseUnistdTable(url string) ([]*syscallEntry, error) {
	return parseLinuxFile(url, func(line string) (*syscallEntry, error) {
		subs := unistdDefinedRe.FindStringSubmatch(line)
		if subs == nil {
			return nil, nil
		}
		nr, err := strconv.ParseInt(subs[3], 10, 0)
		if err != nil {
			return nil, err
		}
		return &syscallEntry{Arch: "arm64", Number: int(nr), Name: subs[2]}, nil
	})
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

func generateMapCode(amd64, arm64 []*syscallEntry) (string, error) {
	tmpl, err := template.New("utils-syscalls").Parse(outputTemplate)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, templateData{
		Amd64: deduplicateByID(amd64),
		Arm64: deduplicateByID(arm64),
	}); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func writeFileAndFormat(outputPath, content string) error {
	tmpfile, err := os.CreateTemp(path.Dir(outputPath), "utils-syscalls-*")
	if err != nil {
		return err
	}

	if _, err := tmpfile.WriteString(content); err != nil {
		return err
	}

	if err := tmpfile.Close(); err != nil {
		return err
	}

	cmd := exec.Command("gofmt", "-s", "-w", tmpfile.Name())
	if err := cmd.Run(); err != nil {
		return err
	}

	return os.Rename(tmpfile.Name(), outputPath)
}
