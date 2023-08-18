// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ebpf

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"sort"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/process/util"
)

type existCache struct {
	mtx  sync.Mutex
	path string
	c    map[string]bool
}

func newExistCache(path string) *existCache {
	return &existCache{
		path: path,
		c:    make(map[string]bool),
	}
}

var funcCache = newExistCache("/proc/kallsyms")

// VerifyKernelFuncs ensures all kernel functions exist in ksyms located at provided path.
func VerifyKernelFuncs(requiredKernelFuncs ...string) (map[string]struct{}, error) {
	return funcCache.verifyKernelFuncs(requiredKernelFuncs)
}

func (ec *existCache) verifyKernelFuncs(requiredKernelFuncs []string) (map[string]struct{}, error) {
	ec.mtx.Lock()
	defer ec.mtx.Unlock()

	var check util.SSBytes
	for _, rf := range requiredKernelFuncs {
		if _, ok := ec.c[rf]; !ok {
			// only check for functions we don't know about yet
			check = append(check, []byte(rf))
		}
	}

	if len(check) != 0 {
		sort.Sort(check)

		f, err := os.Open(ec.path)
		if err != nil {
			return nil, fmt.Errorf("error reading kallsyms file from: %s: %w", ec.path, err)
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		scanner.Split(bufio.ScanLines)
		for scanner.Scan() {
			fields := bytes.Fields(scanner.Bytes())
			if len(fields) >= 3 {
				if idx := check.Search(fields[2]); idx >= 0 {
					// found it in kallsyms, cache result
					ec.c[string(check[idx])] = true
					check = append(check[:idx], check[idx+1:]...)
				}
			}
		}
		if err := scanner.Err(); err != nil {
			return nil, err
		}
		// anything left in check is missing
		for _, rf := range check {
			ec.c[string(rf)] = false
		}
	}

	// only return missing funcs at this point
	missingStrs := make(map[string]struct{})
	for _, rf := range requiredKernelFuncs {
		if v, ok := ec.c[rf]; !ok || !v {
			missingStrs[rf] = struct{}{}
		}
	}
	return missingStrs, nil
}
