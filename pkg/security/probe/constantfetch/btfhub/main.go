// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/probe/constantfetch"
	utilKernel "github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func main() {
	archivePath := os.Args[1]

	tmpDir, err := os.MkdirTemp("", "extract-dir")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmpDir)

	extractCmd := exec.Command("tar", "xvf", archivePath, "-C", tmpDir)
	if err := extractCmd.Run(); err != nil {
		panic(err)
	}

	archiveFileName := path.Base(archivePath)
	btfFileName := strings.TrimSuffix(archiveFileName, ".tar.xz")
	btfPath := path.Join(tmpDir, btfFileName)

	releasePart := strings.Split(btfFileName, "-")[0]
	kvCode, err := utilKernel.ParseReleaseString(releasePart)
	if err != nil {
		panic(err)
	}
	kv := &kernel.Version{
		Code: kvCode,
	}

	fetcher := NewConstantCollector(btfPath)

	constants, err := probe.GetOffsetConstantsFromFetcher(fetcher, kv)
	if err != nil {
		panic(err)
	}

	for name, value := range constants {
		fmt.Println(name, value)
	}
}

type ConstantCollector struct {
	btfPath   string
	constants map[string]uint64
}

func NewConstantCollector(btfPath string) *ConstantCollector {
	return &ConstantCollector{
		btfPath:   btfPath,
		constants: make(map[string]uint64),
	}
}

var sizeRe = regexp.MustCompile(`size: (\d+), cachelines: \d+, members: \d+`)
var offsetRe = regexp.MustCompile(`/\*\s*(\d+)\s*\d+\s*\*/`)

func (cc *ConstantCollector) AppendSizeofRequest(id, typeName, headerName string) {
	value := parsePaholeOutput(getActualTypeName(typeName), cc.btfPath, func(line string) (uint64, bool) {
		if matches := sizeRe.FindStringSubmatch(line); len(matches) != 0 {
			size, err := strconv.ParseUint(matches[1], 10, 64)
			if err != nil {
				panic(err)
			}
			return size, true
		}
		return 0, false
	})
	cc.constants[id] = value
}

func (cc *ConstantCollector) AppendOffsetofRequest(id, typeName, fieldName, headerName string) {
	value := parsePaholeOutput(getActualTypeName(typeName), cc.btfPath, func(line string) (uint64, bool) {
		if strings.Contains(line, fieldName) {
			if matches := offsetRe.FindStringSubmatch(line); len(matches) != 0 {
				size, err := strconv.ParseUint(matches[1], 10, 64)
				if err != nil {
					panic(err)
				}
				return size, true
			}
		}
		return 0, false
	})
	cc.constants[id] = value
}

func (c *ConstantCollector) FinishAndGetResults() (map[string]uint64, error) {
	return c.constants, nil
}

func getActualTypeName(tn string) string {
	prefixes := []string{"struct", "enum"}
	for _, prefix := range prefixes {
		tn = strings.TrimPrefix(tn, prefix+" ")
	}
	return tn
}

func parsePaholeOutput(tyName, btfPath string, lineF func(string) (uint64, bool)) uint64 {
	var btfArg string
	if btfPath != "" {
		btfArg = fmt.Sprintf("--btf_base=%s", btfPath)
	}
	cmd := exec.Command("pahole", tyName, btfArg)
	cmd.Stdin = os.Stdin
	output, err := cmd.Output()
	if err != nil {
		exitErr := err.(*exec.ExitError)
		fmt.Println(string(exitErr.Stderr))
		panic(err)
	}

	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := scanner.Text()
		value, ok := lineF(line)
		if ok {
			return value
		}
	}
	return constantfetch.ErrorSentinel
}
