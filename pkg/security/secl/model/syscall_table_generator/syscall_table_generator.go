// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package main holds main related files
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
	"strconv"
	"strings"
	"text/template"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

func main() {
	var (
		inputTableURL      string
		outputEnumPath     string
		outputStringerPath string
		abis               string
	)

	flag.StringVar(&inputTableURL, "table-url", "", "URL of the table to use for the generation")
	flag.StringVar(&outputEnumPath, "output", "", "Output path of the generated file with the constant declarations")
	flag.StringVar(&outputStringerPath, "output-string", "", "Output path of the generated file with the stringer code")
	flag.StringVar(&abis, "abis", "", "Comma separated list of ABIs to keep")
	flag.Parse()

	if inputTableURL == "" || outputEnumPath == "" || outputStringerPath == "" {
		fmt.Fprintf(os.Stderr, "Please provide required flags\n")
		flag.Usage()
		return
	}

	abiList := strings.Split(abis, ",")

	var (
		syscalls []*syscallDefinition
		err      error
	)

	if strings.HasSuffix(inputTableURL, ".tbl") {
		syscalls, err = parseSyscallTable(inputTableURL, abiList)
	} else {
		syscalls, err = parseUnistdTable(inputTableURL)
	}

	if err != nil {
		panic(err)
	}

	outputContent, err := generateEnumCode(syscalls)
	if err != nil {
		panic(err)
	}

	if err := writeFileAndFormat(outputEnumPath, outputContent); err != nil {
		panic(err)
	}

	if err := generateStringer(outputEnumPath, outputStringerPath); err != nil {
		panic(err)
	}
}

type syscallDefinition struct {
	Number        int
	Abi           string
	Name          string
	CamelCaseName string
}

func parseLinuxFile(url string, perLine func(string) (*syscallDefinition, error)) ([]*syscallDefinition, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
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

func parseUnistdTable(url string) ([]*syscallDefinition, error) {
	return parseLinuxFile(url, func(line string) (*syscallDefinition, error) {
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

func parseSyscallTable(url string, abis []string) ([]*syscallDefinition, error) {
	return parseLinuxFile(url, func(line string) (*syscallDefinition, error) {
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

		if containsStringSlice(abis, abi) {
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
	tmpfile, err := os.CreateTemp(path.Dir(outputPath), "syscalls-enum")
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

func generateStringer(inputPath, outputPath string) error {
	return exec.Command("go", "run", "golang.org/x/tools/cmd/stringer", "-type", "Syscall", "-output", outputPath, inputPath).Run()
}

func containsStringSlice(slice []string, value string) bool {
	for _, current := range slice {
		if current == value {
			return true
		}
	}
	return false
}
