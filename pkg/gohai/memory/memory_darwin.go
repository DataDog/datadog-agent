// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

package memory

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

func getMemoryInfo() (memoryInfo map[string]string, err error) {
	memoryInfo = make(map[string]string)

	out, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
	if err == nil {
		memoryInfo["total"] = strings.Trim(string(out), "\n")
	}

	out, err = exec.Command("sysctl", "-n", "vm.swapusage").Output()
	if err == nil {
		swap := regexp.MustCompile("total = ").Split(string(out), 2)[1]
		memoryInfo["swap_total"] = strings.Split(swap, " ")[0]
	}

	return
}

func getMemoryInfoByte() (uint64, uint64, []string, error) {
	memInfo, err := getMemoryInfo()
	var mem, swap uint64
	warnings := []string{}

	// mem is already in bytes but `swap_total` use the format "5120,00M"
	if v, ok := memInfo["swap_total"]; ok {
		idx := strings.IndexAny(v, ",.") // depending on the locale either a comma or dot is used
		swapTotal, e := strconv.ParseUint(v[0:idx], 10, 64)
		if e == nil {
			swap = swapTotal * 1024 * 1024 // swapTotal is in mb
		} else {
			warnings = append(warnings, fmt.Sprintf("could not parse swap size: %s", e))
		}
	}

	if v, ok := memInfo["total"]; ok {
		t, e := strconv.ParseUint(v, 10, 64)
		if e == nil {
			mem = t // mem is returned in bytes
		} else {
			warnings = append(warnings, fmt.Sprintf("could not parse memory size: %s", e))
		}
	}

	return mem, swap, warnings, err
}
