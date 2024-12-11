// This file is licensed under the MIT License.
//
// Copyright (c) 2017 Nathan Sweet
// Copyright (c) 2018, 2019 Cloudflare
// Copyright (c) 2019 Authors of Cilium
//
// MIT License
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

//go:build linux

package kernel

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/funcs"
)

// PossibleCPUs returns the max number of CPUs a system may possibly have
// Logical CPU numbers must be of the form 0-n
var PossibleCPUs = funcs.Memoize(func() (int, error) {
	return parseCPUSingleRangeFromFile("/sys/devices/system/cpu/possible")
})

// OnlineCPUs returns the individual CPU numbers that are online for the system
var OnlineCPUs = funcs.Memoize(func() ([]uint, error) {
	return parseCPUMultipleRangeFromFile("/sys/devices/system/cpu/online")
})

func parseCPUMultipleRangeFromFile(path string) ([]uint, error) {
	spec, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	n, err := parseCPUMultipleRange(string(spec))
	if err != nil {
		return nil, fmt.Errorf("can't parse %s: %v", path, err)
	}

	return n, nil
}

func parseCPUSingleRangeFromFile(path string) (int, error) {
	spec, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}

	n, err := parseCPUSingleRange(string(spec))
	if err != nil {
		return 0, fmt.Errorf("can't parse %s: %v", path, err)
	}

	return n, nil
}

// parseCPUSingleRange parses the number of cpus from a string produced
// by bitmap_list_string() in the Linux kernel.
// Multiple ranges are rejected, since they can't be unified
// into a single number.
// This is the format of /sys/devices/system/cpu/possible, it
// is not suitable for /sys/devices/system/cpu/online, etc.
func parseCPUSingleRange(spec string) (int, error) {
	if strings.Trim(spec, "\n") == "0" {
		return 1, nil
	}

	var low, high int
	n, err := fmt.Sscanf(spec, "%d-%d\n", &low, &high)
	if n != 2 || err != nil {
		return 0, fmt.Errorf("invalid format: %s", spec)
	}
	if low != 0 {
		return 0, fmt.Errorf("CPU spec doesn't start at zero: %s", spec)
	}

	// cpus is 0 indexed
	return high + 1, nil
}

func parseCPUMultipleRange(spec string) ([]uint, error) {
	var cpus []uint
	cpuStr := strings.Trim(spec, "\n")
	for _, r := range strings.Split(cpuStr, ",") {
		parts := strings.SplitN(r, "-", 2)
		low, err := strconv.ParseUint(parts[0], 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid format: %s", spec)
		}
		if len(parts) == 1 {
			cpus = append(cpus, uint(low))
			continue
		}

		high, err := strconv.ParseUint(parts[1], 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid format: %s", spec)
		}
		for i := low; i <= high; i++ {
			cpus = append(cpus, uint(i))
		}
	}
	return cpus, nil
}
