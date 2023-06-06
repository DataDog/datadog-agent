// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ebpfcheck

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
)

const bpfProgPrefix = "bpf_prog_"

func kallsymsBPFPrograms() (map[string][]string, error) {
	f, err := os.Open("/proc/kallsyms")
	if err != nil {
		return nil, fmt.Errorf("error reading kallsyms file: %w", err)
	}
	defer f.Close()

	tagToFunctions := make(map[string][]string)
	scanner := bufio.NewScanner(f)
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		fields := bytes.Fields(scanner.Bytes())
		if len(fields) >= 3 {
			fnName := fields[2]
			if bytes.HasPrefix(fnName, []byte(bpfProgPrefix)) {
				parts := bytes.SplitN(fnName[len(bpfProgPrefix):], []byte("_"), 2)
				if len(parts) == 2 {
					tag, name := string(parts[0]), string(parts[1])
					if names, ok := tagToFunctions[tag]; ok {
						for _, n := range names {
							// we don't want to store duplicate function names
							if n == name {
								continue
							}
						}
						tagToFunctions[tag] = append(names, name)
					} else {
						tagToFunctions[tag] = []string{name}
					}
				}
			}
		}
	}
	return tagToFunctions, nil
}
