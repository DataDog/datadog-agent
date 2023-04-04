// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"bufio"
	_ "embed"
	"flag"
	"os"
	"os/exec"
	"regexp"
	"text/template"
)

//go:embed gen.go.tmpl
var templateSrc string

type mapEntry struct {
	Name        string
	TrimmedName string
	Kind        string
}

type tmplContext struct {
	PackageName string
	Entries     []mapEntry
}

const BPF_MAX_OBJ_SIZE = 15 // 16 - 1 for the \0

func main() {
	var (
		runtimePath string
		outputPath  string
		packageName string
	)

	flag.StringVar(&runtimePath, "runtime-path", "", "path to the runtime generated path")
	flag.StringVar(&outputPath, "output", "", "Output path of the generated file with the map names")
	flag.StringVar(&packageName, "pkg-name", "", "Package name to use in the output")
	flag.Parse()

	mapMatcher := regexp.MustCompile(`BPF_(.*?)_MAP\(\s*(.*?)\s*,.*?\)`)
	defineMatcher := regexp.MustCompile(`\s*#define BPF`)

	f, err := os.Open(runtimePath)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)

	entries := make([]mapEntry, 0)
	for scanner.Scan() {
		line := scanner.Text()
		if defineMatcher.MatchString(line) {
			continue
		}

		submatches := mapMatcher.FindAllStringSubmatch(line, -1)
		for _, submatch := range submatches {
			kind := submatch[1]
			name := submatch[2]

			trimmed := name
			if len(name) > BPF_MAX_OBJ_SIZE {
				trimmed = name[:BPF_MAX_OBJ_SIZE]
			}

			entries = append(entries, mapEntry{
				Name:        name,
				TrimmedName: trimmed,
				Kind:        kind,
			})
		}
	}
	if err := scanner.Err(); err != nil {
		panic(err)
	}

	tmpl, err := template.New("bpf_maps").Parse(templateSrc)
	if err != nil {
		panic(err)
	}

	outputFile, err := os.Create(outputPath)
	if err != nil {
		panic(err)
	}

	if err := tmpl.Execute(outputFile, tmplContext{
		PackageName: packageName,
		Entries:     entries,
	}); err != nil {
		panic(err)
	}

	if err := outputFile.Close(); err != nil {
		panic(err)
	}

	cmd := exec.Command("gofmt", "-s", "-w", outputPath)
	if err := cmd.Run(); err != nil {
		panic(err)
	}
}
