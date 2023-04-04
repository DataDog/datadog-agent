// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
)

type mapEntry struct {
	Name string
	Kind string
}

func main() {
	prebuiltPath := "pkg/ebpf/bytecode/build/runtime/runtime-security.c"
	mapMatcher := regexp.MustCompile(`BPF_(.*?)_MAP\(\s*(.*?)\s*,.*?\)`)
	defineMatcher := regexp.MustCompile(`\s*#define BPF`)

	f, err := os.Open(prebuiltPath)
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

	fmt.Println(entries)
}
