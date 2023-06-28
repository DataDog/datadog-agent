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

	"github.com/DataDog/datadog-agent/pkg/gohai/utils"
)

func getTotalBytes() (uint64, error) {
	out, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
	if err != nil {
		return 0, fmt.Errorf("sysctl: %w", err)
	}

	v := strings.Trim(string(out), "\n")
	mem, e := strconv.ParseUint(v, 10, 64)
	if e != nil {
		return 0, fmt.Errorf("could not parse memory size: %w", e)
	}

	return mem, nil // mem is in bytes
}

func getTotalSwapKb() (uint64, error) {
	out, err := exec.Command("sysctl", "-n", "vm.swapusage").Output()
	if err != nil {
		return 0, fmt.Errorf("sysctl: %w", err)
	}

	swap := regexp.MustCompile("total = ").Split(string(out), 2)[1]
	v := strings.Split(swap, " ")[0]
	idx := strings.IndexAny(v, ",.") // depending on the locale either a comma or dot is used
	swapTotal, err := strconv.ParseUint(v[0:idx], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("could not parse swap size: %w", err)
	}

	return swapTotal * 1024, nil // swapTotal is in mb
}

func (info *Info) fillMemoryInfo() {
	info.TotalBytes = utils.NewValueFrom(getTotalBytes())
	info.SwapTotalKb = utils.NewValueFrom(getTotalSwapKb())
}
