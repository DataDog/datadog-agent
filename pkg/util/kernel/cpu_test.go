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
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/exp/slices"
)

func TestParseCPUSingleRange(t *testing.T) {
	for str, result := range map[string]int{
		"0-1":   2,
		"0-2\n": 3,
		"0":     1,
	} {
		n, err := parseCPUSingleRange(str)
		if err != nil {
			t.Errorf("Can't parse `%s`: %v", str, err)
		} else if n != result {
			t.Error("Parsing", str, "returns", n, "instead of", result)
		}
	}

	for _, str := range []string{
		"0,3-4",
		"0-",
		"1,",
		"",
	} {
		_, err := parseCPUSingleRange(str)
		if err == nil {
			t.Error("Parsed invalid format:", str)
		}
	}
}

func TestParseCPUMultipleRange(t *testing.T) {
	for str, result := range map[string][]uint{
		"0-1":       {0, 1},
		"0-2\n":     {0, 1, 2},
		"0":         {0},
		"0,2-4":     {0, 2, 3, 4},
		"0-2,4-6,8": {0, 1, 2, 4, 5, 6, 8},
	} {
		n, err := parseCPUMultipleRange(str)
		if err != nil {
			t.Errorf("Can't parse `%s`: %v", str, err)
		} else if !slices.Equal(n, result) {
			t.Error("Parsing", str, "returns", n, "instead of", result)
		}
	}

	for _, str := range []string{
		"-",
		"",
		"a",
	} {
		_, err := parseCPUMultipleRange(str)
		if err == nil {
			t.Error("Parsed invalid format:", str)
		}
	}
}

func TestOnlineCPU(t *testing.T) {
	cpus, err := OnlineCPUs()
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(cpus), 1)
}

func TestPossibleCPU(t *testing.T) {
	cpus, err := PossibleCPUs()
	require.NoError(t, err)
	require.GreaterOrEqual(t, cpus, 1)
}
