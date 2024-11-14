// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ebpf

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/process/util"
)

type existCache struct {
	mtx  sync.Mutex
	path string
	c    map[string]uint64
}

func newExistCache(path string) *existCache {
	return &existCache{
		path: path,
		c:    make(map[string]uint64),
	}
}

const invalidAddress = 0xffffffffffffffff

var funcCache = newExistCache("/proc/kallsyms")

// GetKernelSymbolsAddressesNoCache returns the requested kernel symbols and addresses without using the cache
// It expects a reader from which to read the kernel symbols.
func GetKernelSymbolsAddressesNoCache(ksymsReader io.Reader, ksyms ...string) (map[string]uint64, error) {
	var check util.SSBytes
	for _, rf := range ksyms {
		check = append(check, []byte(rf))
	}

	present := make(map[string]uint64, len(ksyms))
	if err := findKernelFuncs(ksymsReader, func(ksym string, addr uint64) {
		if addr != invalidAddress {
			present[ksym] = addr
		}
	}, check); err != nil {
		return nil, err
	}

	var errs []error
	for _, sym := range ksyms {
		if _, ok := present[sym]; !ok {
			errs = append(errs, fmt.Errorf("failed to get address of symbol %s", sym))
		}
	}

	return present, errors.Join(errs...)
}

// VerifyKernelFuncs ensures all kernel functions exist in ksyms located at provided path.
func VerifyKernelFuncs(requiredKernelFuncs ...string) (map[string]struct{}, error) {
	return funcCache.verifyKernelFuncs(requiredKernelFuncs)
}

func (ec *existCache) verifyKernelFuncs(requiredKernelFuncs []string) (map[string]struct{}, error) {
	ec.mtx.Lock()
	defer ec.mtx.Unlock()

	f, err := os.Open(ec.path)
	if err != nil {
		return nil, fmt.Errorf("error reading kallsyms file from: %s: %w", ec.path, err)
	}
	defer f.Close()

	var check util.SSBytes
	for _, rf := range requiredKernelFuncs {
		if _, ok := ec.c[rf]; !ok {
			// only check for functions we don't know about yet
			check = append(check, []byte(rf))
		}
	}

	if err := findKernelFuncs(f, func(ksym string, addr uint64) {
		ec.c[ksym] = addr
	}, check); err != nil {
		return nil, err
	}

	// only return missing funcs at this point
	missingStrs := make(map[string]struct{})
	for _, rf := range requiredKernelFuncs {
		if v, ok := ec.c[rf]; !ok || (v == invalidAddress) {
			missingStrs[rf] = struct{}{}
		}
	}
	return missingStrs, nil
}

func findKernelFuncs(ksymsReader io.Reader, writeKsym func(string, uint64), check util.SSBytes) error {
	if len(check) != 0 {
		sort.Sort(check)

		scanner := bufio.NewScanner(ksymsReader)
		scanner.Split(bufio.ScanLines)
		for scanner.Scan() {
			line := scanner.Bytes()

			// if the line doesn't contain any of the functions we're looking for, skip it
			earlyCheck := false
			for _, rf := range check {
				if bytes.Contains(line, rf) {
					earlyCheck = true
					break
				}
			}
			if !earlyCheck {
				continue
			}

			fields := bytes.Fields(line)
			if len(fields) < 2 {
				continue
			}

			if idx := check.Search(fields[2]); idx >= 0 {
				s, err := strconv.ParseUint(string(fields[0]), 16, 64)
				if err != nil {
					return fmt.Errorf("failed to parse kallsyms address for symbol %s: %w", string(fields[2]), err)
				}

				writeKsym(string(check[idx]), s)
				check = append(check[:idx], check[idx+1:]...)
			}
		}
		if err := scanner.Err(); err != nil {
			return err
		}
		// anything left in check is missing
		for _, rf := range check {
			writeKsym(string(rf), invalidAddress)
		}
	}

	return nil
}
