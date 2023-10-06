// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

package memory

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/gohai/utils"
	log "github.com/cihub/seelog"
)

func parseMemoryInfo(reader io.Reader) (totalBytes utils.Value[uint64], swapTotalKb utils.Value[uint64], err error) {
	var lines []string
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if scanner.Err() != nil {
		err = fmt.Errorf("could not read /proc/meminfo: %w", scanner.Err())
		return
	}

	totalBytes = utils.NewErrorValue[uint64](fmt.Errorf("'MemTotal' not found in /proc/meminfo"))
	swapTotalKb = utils.NewErrorValue[uint64](fmt.Errorf("'SwapTotal' not found in /proc/meminfo"))
	for _, line := range lines {
		key, valUnit, found := strings.Cut(line, ":")
		if !found {
			log.Warnf("/proc/meminfo has line with unexpected format: \"%s\"", line)
			continue
		}

		value, _, found := strings.Cut(strings.TrimSpace(valUnit), " ")
		if !found {
			continue
		}

		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		switch key {
		case "MemTotal":
			val, parseErr := strconv.ParseUint(value, 10, 64)
			if parseErr == nil {
				// val is in kb
				totalBytes = utils.NewValue(val * 1024)
			} else {
				totalBytes = utils.NewErrorValue[uint64](fmt.Errorf("could not parse total size: %w", parseErr))
			}
		case "SwapTotal":
			val, parseErr := strconv.ParseUint(value, 10, 64)
			if parseErr == nil {
				swapTotalKb = utils.NewValue(val)
			} else {
				swapTotalKb = utils.NewErrorValue[uint64](fmt.Errorf("could not parse total swap size: %w", parseErr))
			}
		}
	}

	return
}

func (info *Info) fillMemoryInfo() {
	var totalBytes, swapTotalKb utils.Value[uint64]

	file, err := os.Open("/proc/meminfo")
	if err == nil {
		defer file.Close()
		totalBytes, swapTotalKb, err = parseMemoryInfo(file)
	} else {
		err = fmt.Errorf("could not open /proc/meminfo: %w", err)
	}

	if err != nil {
		totalBytes = utils.NewErrorValue[uint64](err)
		swapTotalKb = utils.NewErrorValue[uint64](err)
	}

	info.TotalBytes = totalBytes
	info.SwapTotalKb = swapTotalKb
}
