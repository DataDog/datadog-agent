// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package main generates pkg/security/secl/model/syscalls_linux_{amd64,arm64}.go
// (and the matching stringer files) from upstream Linux kernel syscall tables.
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
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"text/template"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

func main() {
	var (
		inputTablePath     string
		outputEnumPath     string
		outputStringerPath string
		stringerBin        string
		abis               string
	)

	flag.StringVar(&inputTablePath, "table-file", "", "Path to the kernel syscall table (.tbl or unistd.h)")
	flag.StringVar(&outputEnumPath, "output", "", "Output path of the generated file with the constant declarations")
	flag.StringVar(&outputStringerPath, "output-string", "", "Output path of the generated file with the stringer code")
	flag.StringVar(&stringerBin, "stringer", "", "Path to a stringer binary (default: go run golang.org/x/tools/cmd/stringer)")
	flag.StringVar(&abis, "abis", "", "Comma separated list of ABIs to keep (only used for .tbl files)")
	flag.Parse()

	if inputTablePath == "" || outputEnumPath == "" || outputStringerPath == "" {
		fmt.Fprintf(os.Stderr, "Please provide required flags\n")
		flag.Usage()
		os.Exit(1)
	}

	abiList := strings.Split(abis, ",")

	var (
		syscalls []*syscallDefinition
		err      error
	)

	// http_file repos stage the content as ".../file/downloaded" with no
	// extension, so sniff the contents instead of trusting the path suffix.
	isTbl, err := looksLikeSyscallTbl(inputTablePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read table: %v\n", err)
		os.Exit(1)
	}
	if isTbl {
		syscalls, err = parseSyscallTable(inputTablePath, abiList)
	} else {
		syscalls, err = parseUnistdTable(inputTablePath)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse table: %v\n", err)
		os.Exit(1)
	}

	outputContent, err := generateEnumCode(syscalls)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to generate enum: %v\n", err)
		os.Exit(1)
	}

	if err := writeFileAndFormat(outputEnumPath, outputContent); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write enum: %v\n", err)
		os.Exit(1)
	}

	if err := generateStringer(stringerBin, outputEnumPath, outputStringerPath); err != nil {
		fmt.Fprintf(os.Stderr, "failed to generate stringer: %v\n", err)
		os.Exit(1)
	}
}

type syscallDefinition struct {
	Number        int
	Abi           string
	Name          string
	CamelCaseName string
}

func parseLinuxFile(path string, perLine func(string) (*syscallDefinition, error)) ([]*syscallDefinition, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parseReader(f, perLine)
}

// looksLikeSyscallTbl reports whether path is an x86 syscall_*.tbl (tab-separated
// number/abi/name lines) rather than an asm-generic unistd.h.
func looksLikeSyscallTbl(path string) (bool, error) {
	if strings.HasSuffix(path, ".tbl") {
		return true, nil
	}
	if strings.HasSuffix(path, ".h") {
		return false, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) >= 3 {
			if _, err := strconv.ParseInt(parts[0], 10, 0); err == nil {
				return true, nil
			}
		}
		return false, nil
	}
	return false, scanner.Err()
}

func parseReader(r io.Reader, perLine func(string) (*syscallDefinition, error)) ([]*syscallDefinition, error) {
	scanner := bufio.NewScanner(r)
	syscalls := make([]*syscallDefinition, 0)

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		def, err := perLine(trimmed)
		if err != nil {
			return nil, err
		}

		if def != nil {
			syscalls = append(syscalls, def)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return syscalls, nil
}

var unistdDefinedRe = regexp.MustCompile(`#define __NR(3264)?_([0-9a-zA-Z_][0-9a-zA-Z_]*)\s+([0-9]+)`)

func parseUnistdTable(path string) ([]*syscallDefinition, error) {
	return parseLinuxFile(path, func(line string) (*syscallDefinition, error) {
		subs := unistdDefinedRe.FindStringSubmatch(line)
		if subs != nil {
			name := subs[2]
			nr, err := strconv.ParseInt(subs[3], 10, 0)
			if err != nil {
				return nil, err
			}

			camelCaseName := snakeToCamelCase(name)

			return &syscallDefinition{
				Number:        int(nr),
				Name:          name,
				CamelCaseName: camelCaseName,
			}, nil
		}
		return nil, nil
	})
}

func parseSyscallTable(path string, abis []string) ([]*syscallDefinition, error) {
	return parseLinuxFile(path, func(line string) (*syscallDefinition, error) {
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
		camelCaseName := snakeToCamelCase(name)

		if slices.Contains(abis, abi) {
			return &syscallDefinition{
				Number:        int(number),
				Abi:           abi,
				Name:          name,
				CamelCaseName: camelCaseName,
			}, nil
		}
		return nil, nil
	})
}

const outputTemplateContent = `
// Code generated - DO NOT EDIT.
// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package model

// Syscall represents a syscall identifier
type Syscall int

// Linux syscall identifiers
const (
	{{- range .}}
	Sys{{.CamelCaseName}} Syscall = {{.Number}}
	{{- end}}
)
`

func generateEnumCode(syscalls []*syscallDefinition) (string, error) {
	tmpl, err := template.New("enum-code").Parse(outputTemplateContent)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, syscalls); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func snakeToCamelCase(snake string) string {
	parts := strings.Split(snake, "_")
	caser := cases.Title(language.English)
	var b strings.Builder
	for _, part := range parts {
		b.WriteString(caser.String(part))
	}

	return b.String()
}

func writeFileAndFormat(outputPath string, content string) error {
	formatted, err := format.Source([]byte(content))
	if err != nil {
		return err
	}
	return os.WriteFile(outputPath, formatted, 0o644)
}

// generateStringer runs stringer against a one-file directory. rules_go's
// @go_stringer is patched to ImportDir the whole package with UseAllFiles, so
// pointing it at pkg/security/secl/model would see both amd64 and arm64
// Syscall definitions. Isolating the enum in a temp dir avoids that.
func generateStringer(stringerBin, inputPath, outputPath string) error {
	tmp, err := os.MkdirTemp("", "syscall-stringer-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	src, err := os.ReadFile(inputPath)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(tmp, "syscalls.go"), src, 0o644); err != nil {
		return err
	}

	absOut, err := filepath.Abs(outputPath)
	if err != nil {
		return err
	}

	var cmd *exec.Cmd
	if stringerBin != "" {
		cmd = exec.Command(stringerBin, "-type", "Syscall", "-tags", "linux", "-output", absOut, tmp)
	} else {
		cmd = exec.Command("go", "run", "golang.org/x/tools/cmd/stringer", "-type", "Syscall", "-tags", "linux", "-output", absOut, tmp)
	}
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	return cmd.Run()
}
