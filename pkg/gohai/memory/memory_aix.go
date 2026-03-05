// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

//go:build aix

package memory

import (
	"os/exec"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/gohai/utils"
)

func (info *Info) fillMemoryInfo() {
	info.TotalBytes = utils.NewErrorValue[uint64](utils.ErrNotCollectable)
	info.SwapTotalKb = utils.NewErrorValue[uint64](utils.ErrNotCollectable)

	out, err := exec.Command("prtconf").Output()
	if err != nil {
		return
	}

	for _, line := range strings.Split(string(out), "\n") {
		key, val, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)

		if key == "Memory Size" {
			// e.g. "8192 Megabytes" or "8 Gigabytes" or "16384 MB"
			fields := strings.Fields(val)
			if len(fields) >= 2 {
				n, parseErr := strconv.ParseUint(fields[0], 10, 64)
				if parseErr == nil {
					unit := strings.ToLower(fields[1])
					var bytes uint64
					switch {
					case strings.HasPrefix(unit, "g"):
						bytes = n * 1024 * 1024 * 1024
					case strings.HasPrefix(unit, "m"):
						bytes = n * 1024 * 1024
					case strings.HasPrefix(unit, "k"):
						bytes = n * 1024
					default:
						bytes = n
					}
					info.TotalBytes = utils.NewValue(bytes)
				}
			}
		}
	}

	// Collect swap from lsps -s: "Total Paging Space   Percent Used\n    2048MB                1%"
	swapOut, swapErr := exec.Command("lsps", "-s").Output()
	if swapErr == nil {
		lines := strings.Split(strings.TrimSpace(string(swapOut)), "\n")
		if len(lines) >= 2 {
			fields := strings.Fields(lines[1])
			if len(fields) >= 1 {
				sizeStr := strings.ToLower(fields[0])
				var swapKb uint64
				var parseErr error
				switch {
				case strings.HasSuffix(sizeStr, "gb"):
					n, err := strconv.ParseUint(strings.TrimSuffix(sizeStr, "gb"), 10, 64)
					parseErr = err
					swapKb = n * 1024 * 1024
				case strings.HasSuffix(sizeStr, "mb"):
					n, err := strconv.ParseUint(strings.TrimSuffix(sizeStr, "mb"), 10, 64)
					parseErr = err
					swapKb = n * 1024
				case strings.HasSuffix(sizeStr, "kb"):
					n, err := strconv.ParseUint(strings.TrimSuffix(sizeStr, "kb"), 10, 64)
					parseErr = err
					swapKb = n
				}
				if parseErr == nil && swapKb > 0 {
					info.SwapTotalKb = utils.NewValue(swapKb)
				}
			}
		}
	}
}
