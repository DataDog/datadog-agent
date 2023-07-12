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

package kernel

import (
	"fmt"
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/funcs"
)

// PossibleCPUs returns the max number of CPUs a system may possibly have
// Logical CPU numbers must be of the form 0-n
var PossibleCPUs = funcs.Memoize(func() (int, error) {
	return parseCPUsFromFile("/sys/devices/system/cpu/possible")
})

func parseCPUsFromFile(path string) (int, error) {
	spec, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}

	n, err := parseCPUs(string(spec))
	if err != nil {
		return 0, fmt.Errorf("can't parse %s: %v", path, err)
	}

	return n, nil
}

// parseCPUs parses the number of cpus from a string produced
// by bitmap_list_string() in the Linux kernel.
// Multiple ranges are rejected, since they can't be unified
// into a single number.
// This is the format of /sys/devices/system/cpu/possible, it
// is not suitable for /sys/devices/system/cpu/online, etc.
func parseCPUs(spec string) (int, error) {
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
