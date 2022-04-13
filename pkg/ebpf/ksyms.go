// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ebpf

import (
	"bufio"
	"os"
	"sort"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/pkg/errors"
)

// VerifyKernelFuncs ensures all kernel functions exist in ksyms located at provided path.
func VerifyKernelFuncs(path string, requiredKernelFuncs []string) ([]string, error) {
	missing := make(util.SSBytes, len(requiredKernelFuncs))
	for i, f := range requiredKernelFuncs {
		missing[i] = []byte(f)
	}
	sort.Sort(missing)

	f, err := os.Open(path)
	if err != nil {
		return nil, errors.Wrapf(err, "error reading kallsyms file from: %s", path)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Split(bufio.ScanWords)
	for scanner.Scan() && len(missing) > 0 {
		if i := missing.Search(scanner.Bytes()); i < len(missing) {
			missing = append(missing[:i], missing[i+1:]...)
		}
	}

	missingStrs := make([]string, len(missing))
	for i := range missing {
		missingStrs[i] = string(missing[i])
	}

	return missingStrs, nil
}
