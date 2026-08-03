// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package numamonitoring

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// parseNUMAMaps aggregates resident bytes by NUMA node. numa_maps reports page
// counts; kernelpagesize_kB, when present, applies to the current mapping.
func parseNUMAMaps(reader io.Reader, basePageSize uint64) (map[int]uint64, error) {
	resident := make(map[int]uint64)
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		pageSize := basePageSize
		for _, field := range fields {
			if value, ok := strings.CutPrefix(field, "kernelpagesize_kB="); ok {
				kb, err := strconv.ParseUint(value, 10, 64)
				if err != nil {
					return nil, fmt.Errorf("parse kernel page size %q: %w", value, err)
				}
				pageSize = kb * 1024
			}
		}

		for _, field := range fields {
			if len(field) < 4 || field[0] != 'N' {
				continue
			}
			nodeValue, pagesValue, ok := strings.Cut(field[1:], "=")
			if !ok {
				continue
			}
			node, err := strconv.Atoi(nodeValue)
			if err != nil {
				continue
			}
			pages, err := strconv.ParseUint(pagesValue, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("parse NUMA page count %q: %w", pagesValue, err)
			}
			resident[node] += pages * pageSize
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan numa_maps: %w", err)
	}
	return resident, nil
}

func parseRangeList(value string) ([]int, error) {
	var result []int
	for _, item := range strings.Split(strings.TrimSpace(value), ",") {
		if item == "" {
			continue
		}
		startValue, endValue, rangeValue := strings.Cut(item, "-")
		start, err := strconv.Atoi(startValue)
		if err != nil {
			return nil, fmt.Errorf("parse range %q: %w", item, err)
		}
		end := start
		if rangeValue {
			end, err = strconv.Atoi(endValue)
			if err != nil || end < start {
				return nil, fmt.Errorf("parse range %q", item)
			}
		}
		for current := start; current <= end; current++ {
			result = append(result, current)
		}
	}
	return result, nil
}
