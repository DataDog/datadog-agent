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

func main() {
	prebuiltPath := "pkg/ebpf/bytecode/build/runtime/runtime-security.c"
	mapMatcher := regexp.MustCompile(`BPF_(.*?)_MAP\((.*?),.*?\)`)
	defineMatcher := regexp.MustCompile(`\s*#define BPF`)

	f, err := os.Open(prebuiltPath)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := scanner.Text()
		if defineMatcher.MatchString(line) {
			continue
		}

		submatches := mapMatcher.FindAllStringSubmatch(line, -1)
		for _, submatch := range submatches {
			kind := submatch[1]
			name := submatch[2]
			fmt.Println(kind, name)
		}
	}

	if err := scanner.Err(); err != nil {
		panic(err)
	}
}
