// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ebpf

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/process/util"
)

// VerifyKernelFuncs ensures all kernel functions exist in ksyms located at provided path.
func VerifyKernelFuncs(path string, requiredKernelFuncs []string) (map[string]struct{}, error) {
	missing := make(util.SSBytes, len(requiredKernelFuncs))
	for i, f := range requiredKernelFuncs {
		missing[i] = []byte(f)
	}
	sort.Sort(missing)

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("error reading kallsyms file from: %s: %w", path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Split(bufio.ScanWords)
	for scanner.Scan() && len(missing) > 0 {
		if i := missing.Search(scanner.Bytes()); i < len(missing) {
			missing = append(missing[:i], missing[i+1:]...)
		}
	}

	missingStrs := make(map[string]struct{}, len(missing))
	for i := range missing {
		missingStrs[string(missing[i])] = struct{}{}
	}
	return missingStrs, nil
}

func GetSymbolsAddresses(path string, symbols []string) (map[string]uint64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("error reading kallsyms file from %s: %w", path, err)
	}
	defer f.Close()

	syms := make(map[string]struct{}, len(symbols))
	for _, s := range symbols {
		syms[s] = struct{}{}
	}

	return getSymbolAddress(syms, f)
}

func getSymbolAddress(syms map[string]struct{}, r io.Reader) (map[string]uint64, error) {
	addrs := make(map[string]uint64, len(syms))

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		l := scanner.Text()
		data := strings.Split(l, " ")
		if len(data) != 3 {
			continue
		}

		if _, ok := syms[data[2]]; ok {
			addr, err := strconv.ParseUint(data[0], 16, 64)
			if err == nil {
				addrs[data[2]] = addr
			}
		}

		if len(addrs) == len(syms) {
			break
		}
	}

	if len(addrs) != len(syms) {
		return nil, fmt.Errorf("Failed to get all kernel symbols: %v", addrs)
	}

	return addrs, nil
}
