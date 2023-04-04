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
	"regexp"
	"text/template"
)

//go:embed gen.go.tmpl
var templateSrc string

type mapEntry struct {
	Name string
	Kind string
}

type tmplContext struct {
	PackageName string
	Entries     []mapEntry
}

func main() {
	var (
		runtimePath string
		outputPath  string
	)

	flag.StringVar(&runtimePath, "runtime-path", "", "path to the runtime generated path")
	flag.StringVar(&outputPath, "output", "", "Output path of the generated file with the map names")
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
			entries = append(entries, mapEntry{
				Name: name,
				Kind: kind,
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

	if err := tmpl.Execute(os.Stdout, tmplContext{
		PackageName: "test",
		Entries:     entries,
	}); err != nil {
		panic(err)
	}
}
