// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// codeowners is a struct that holds the owners of the code. Not a full implementation, just
// enough to show owners in KMT tests
type codeowners struct {
	owners map[string]string
}

func loadCodeowners(path string) (*codeowners, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %s", path, err)
	}
	defer f.Close()

	return loadCodeownersWithReader(f), nil
}

func loadCodeownersWithReader(contents io.Reader) *codeowners {
	owners := make(map[string]string)
	scanner := bufio.NewScanner(contents)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.SplitN(line, " ", 2)
		if len(fields) < 2 {
			continue
		}
		pattern := strings.TrimSpace(fields[0])
		pattern = strings.TrimPrefix(pattern, "/")
		patternComponents := strings.Split(pattern, "/")
		lastPatternComponent := patternComponents[len(patternComponents)-1]
		if strings.Contains(lastPatternComponent, "*") || strings.HasSuffix(pattern, ".go") {
			pattern = strings.Join(patternComponents[:len(patternComponents)-1], "/")
		}

		pattern = strings.TrimSuffix(pattern, "/") // Remove trailing slash
		owners[pattern] = strings.TrimSpace(fields[1])
	}

	return &codeowners{owners: owners}
}

func (c *codeowners) matchPackage(pkg string) string {
	segmentLen := len(pkg)
	for segmentLen > 0 {
		if owner, ok := c.owners[pkg[:segmentLen]]; ok {
			return owner
		}
		segmentLen = strings.LastIndex(pkg[:segmentLen], "/")
	}

	return ""
}
