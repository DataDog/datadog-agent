// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ps

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/internal/procinfo"
)

type outputField uint8

const (
	fieldPID outputField = iota
	fieldPPID
	fieldUID
	fieldState
	fieldTTY
	fieldSTime
	fieldTime
	fieldComm
	fieldRSS
	fieldVSZ
	fieldPMem
	fieldPCPU
	fieldETime
)

type fieldDefinition struct {
	name       string
	header     string
	rightAlign bool
	metric     procinfo.Metrics
}

var fieldDefinitions = map[outputField]fieldDefinition{
	fieldPID:   {name: "pid", header: "PID", rightAlign: true},
	fieldPPID:  {name: "ppid", header: "PPID", rightAlign: true},
	fieldUID:   {name: "uid", header: "UID"},
	fieldState: {name: "state", header: "S"},
	fieldTTY:   {name: "tty", header: "TTY"},
	fieldSTime: {name: "stime", header: "STIME", metric: procinfo.MetricStartTime},
	fieldTime:  {name: "time", header: "TIME", rightAlign: true, metric: procinfo.MetricCPUTime},
	fieldComm:  {name: "comm", header: "COMMAND"},
	fieldRSS:   {name: "rss", header: "RSS", rightAlign: true, metric: procinfo.MetricRSS},
	fieldVSZ:   {name: "vsz", header: "VSZ", rightAlign: true, metric: procinfo.MetricVSZ},
	fieldPMem:  {name: "pmem", header: "%MEM", rightAlign: true, metric: procinfo.MetricPMem},
	fieldPCPU:  {name: "pcpu", header: "%CPU", rightAlign: true, metric: procinfo.MetricPCPU},
	fieldETime: {name: "etime", header: "ELAPSED", rightAlign: true, metric: procinfo.MetricElapsed},
}

var fieldsByName = func() map[string]outputField {
	fields := make(map[string]outputField, len(fieldDefinitions)+2)
	for field, definition := range fieldDefinitions {
		fields[definition.name] = field
	}
	// procps documents these spellings as aliases for pcpu and pmem.
	fields["%cpu"] = fieldPCPU
	fields["%mem"] = fieldPMem
	return fields
}()

type sortKey struct {
	field      outputField
	descending bool
}

func parseOutputColumns(formats []string) ([]outputField, error) {
	var columns []outputField
	for _, format := range formats {
		names := splitFieldList(format)
		if names == nil {
			return nil, fmt.Errorf("invalid format: %q", format)
		}
		for _, name := range names {
			field, ok := fieldsByName[strings.ToLower(name)]
			if !ok {
				return nil, fmt.Errorf("unknown format specifier %q", name)
			}
			columns = append(columns, field)
		}
	}
	return columns, nil
}

func parseSortKeys(spec string) ([]sortKey, error) {
	names := splitFieldList(spec)
	if names == nil {
		return nil, fmt.Errorf("invalid sort specification %q", spec)
	}

	keys := make([]sortKey, 0, len(names))
	for _, name := range names {
		descending := false
		switch name[0] {
		case '-':
			descending = true
			name = name[1:]
		case '+':
			name = name[1:]
		}
		if name == "" {
			return nil, fmt.Errorf("invalid sort specification %q", spec)
		}
		field, ok := fieldsByName[strings.ToLower(name)]
		if !ok {
			return nil, fmt.Errorf("unknown sort specifier %q", name)
		}
		keys = append(keys, sortKey{field: field, descending: descending})
	}
	return keys, nil
}

func splitFieldList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}

	var fields []string
	for _, commaPart := range strings.Split(value, ",") {
		if strings.TrimSpace(commaPart) == "" {
			return nil
		}
		fields = append(fields, strings.Fields(commaPart)...)
	}
	return fields
}

func requestedMetrics(columns []outputField, sortKeys []sortKey, full bool) procinfo.Metrics {
	var metrics procinfo.Metrics
	if len(columns) == 0 {
		// TIME is part of both legacy layouts. Full format also contains C and
		// STIME, so request their underlying values without changing columns.
		metrics |= procinfo.MetricCPUTime
		if full {
			metrics |= procinfo.MetricPCPU | procinfo.MetricStartTime
		}
	}
	for _, column := range columns {
		metrics |= fieldDefinitions[column].metric
	}
	for _, key := range sortKeys {
		metrics |= fieldDefinitions[key.field].metric
	}
	return metrics
}

func printCustomProcs(callCtx *builtins.CallContext, procs []procinfo.ProcInfo, columns []outputField) {
	rows := make([][]string, 0, len(procs)+1)
	header := make([]string, len(columns))
	for i, column := range columns {
		header[i] = fieldDefinitions[column].header
	}
	rows = append(rows, header)
	for _, proc := range procs {
		row := make([]string, len(columns))
		for i, column := range columns {
			row[i] = formatField(proc, column)
		}
		rows = append(rows, row)
	}

	widths := make([]int, len(columns))
	for _, row := range rows {
		for i, value := range row {
			widths[i] = max(widths[i], len(value))
		}
	}

	for _, row := range rows {
		for i, value := range row {
			if i > 0 {
				callCtx.Out(" ")
			}
			definition := fieldDefinitions[columns[i]]
			if i == len(row)-1 && !definition.rightAlign {
				callCtx.Out(value)
				continue
			}
			if definition.rightAlign {
				callCtx.Outf("%*s", widths[i], value)
			} else {
				callCtx.Outf("%-*s", widths[i], value)
			}
		}
		callCtx.Out("\n")
	}
}

func formatField(proc procinfo.ProcInfo, field outputField) string {
	switch field {
	case fieldPID:
		return strconv.Itoa(proc.PID)
	case fieldPPID:
		return strconv.Itoa(proc.PPID)
	case fieldUID:
		return valueOrQuestion(proc.UID)
	case fieldState:
		return valueOrQuestion(proc.State)
	case fieldTTY:
		return valueOrQuestion(proc.TTY)
	case fieldSTime:
		if proc.Has(procinfo.MetricStartTime) {
			return valueOrDash(proc.STime)
		}
	case fieldTime:
		if proc.Has(procinfo.MetricCPUTime) {
			return valueOrDash(proc.Time)
		}
	case fieldComm:
		return valueOrQuestion(proc.Cmd)
	case fieldRSS:
		if proc.Has(procinfo.MetricRSS) {
			return strconv.FormatUint(proc.RSSKiB, 10)
		}
	case fieldVSZ:
		if proc.Has(procinfo.MetricVSZ) {
			return strconv.FormatUint(proc.VSZKiB, 10)
		}
	case fieldPMem:
		if proc.Has(procinfo.MetricPMem) && finite(proc.PMem) {
			return fmt.Sprintf("%.1f", proc.PMem)
		}
	case fieldPCPU:
		if proc.Has(procinfo.MetricPCPU) && finite(proc.PCPU) {
			return fmt.Sprintf("%.1f", proc.PCPU)
		}
	case fieldETime:
		if proc.Has(procinfo.MetricElapsed) && proc.Elapsed >= 0 {
			return formatElapsed(proc.Elapsed)
		}
	}
	return "-"
}

func valueOrQuestion(value string) string {
	if value == "" {
		return "?"
	}
	return value
}

func valueOrDash(value string) string {
	if value == "" || value == "?" {
		return "-"
	}
	return value
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func formatElapsed(elapsed time.Duration) string {
	totalSeconds := int64(elapsed / time.Second)
	days := totalSeconds / (24 * 60 * 60)
	hours := (totalSeconds / (60 * 60)) % 24
	minutes := (totalSeconds / 60) % 60
	seconds := totalSeconds % 60
	if days > 0 {
		return fmt.Sprintf("%d-%02d:%02d:%02d", days, hours, minutes, seconds)
	}
	if hours > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
	}
	return fmt.Sprintf("%02d:%02d", minutes, seconds)
}

func sortProcs(procs []procinfo.ProcInfo, keys []sortKey) {
	if len(keys) == 0 || len(procs) < 2 {
		return
	}
	slices.SortStableFunc(procs, func(left, right procinfo.ProcInfo) int {
		for _, key := range keys {
			comparison := compareField(left, right, key.field)
			if comparison.order == 0 {
				continue
			}
			if comparison.unavailable {
				// Unavailable values always sort last, independent of the
				// requested direction.
				return comparison.order
			}
			if key.descending {
				return -comparison.order
			}
			return comparison.order
		}
		return 0
	})
}

type fieldComparison struct {
	order       int
	unavailable bool
}

func compareField(left, right procinfo.ProcInfo, field outputField) fieldComparison {
	switch field {
	case fieldPID:
		return compareValues(left.PID, right.PID, true, true)
	case fieldPPID:
		return compareValues(left.PPID, right.PPID, true, true)
	case fieldUID:
		leftUID, leftErr := strconv.ParseUint(left.UID, 10, 64)
		rightUID, rightErr := strconv.ParseUint(right.UID, 10, 64)
		if leftErr == nil && rightErr == nil {
			return compareValues(leftUID, rightUID, true, true)
		}
		return compareValues(left.UID, right.UID, left.UID != "", right.UID != "")
	case fieldState:
		return compareValues(left.State, right.State, left.State != "", right.State != "")
	case fieldTTY:
		return compareValues(left.TTY, right.TTY, left.TTY != "", right.TTY != "")
	case fieldSTime:
		leftAvailable := left.Has(procinfo.MetricStartTime) && !left.StartTime.IsZero()
		rightAvailable := right.Has(procinfo.MetricStartTime) && !right.StartTime.IsZero()
		return compareValues(
			left.StartTime.UnixNano(),
			right.StartTime.UnixNano(),
			leftAvailable,
			rightAvailable,
		)
	case fieldTime:
		return compareValues(
			left.CPUTime,
			right.CPUTime,
			left.Has(procinfo.MetricCPUTime),
			right.Has(procinfo.MetricCPUTime),
		)
	case fieldComm:
		return compareValues(left.Cmd, right.Cmd, left.Cmd != "", right.Cmd != "")
	case fieldRSS:
		return compareValues(
			left.RSSKiB,
			right.RSSKiB,
			left.Has(procinfo.MetricRSS),
			right.Has(procinfo.MetricRSS),
		)
	case fieldVSZ:
		return compareValues(
			left.VSZKiB,
			right.VSZKiB,
			left.Has(procinfo.MetricVSZ),
			right.Has(procinfo.MetricVSZ),
		)
	case fieldPMem:
		return compareValues(
			left.PMem,
			right.PMem,
			left.Has(procinfo.MetricPMem) && finite(left.PMem),
			right.Has(procinfo.MetricPMem) && finite(right.PMem),
		)
	case fieldPCPU:
		return compareValues(
			left.PCPU,
			right.PCPU,
			left.Has(procinfo.MetricPCPU) && finite(left.PCPU),
			right.Has(procinfo.MetricPCPU) && finite(right.PCPU),
		)
	case fieldETime:
		return compareValues(
			left.Elapsed,
			right.Elapsed,
			left.Has(procinfo.MetricElapsed) && left.Elapsed >= 0,
			right.Has(procinfo.MetricElapsed) && right.Elapsed >= 0,
		)
	}
	return fieldComparison{}
}

func compareValues[T ~int | ~int64 | ~uint64 | ~float64 | ~string](
	left, right T,
	leftAvailable, rightAvailable bool,
) fieldComparison {
	switch {
	case !leftAvailable && !rightAvailable:
		return fieldComparison{}
	case !leftAvailable:
		return fieldComparison{order: 1, unavailable: true}
	case !rightAvailable:
		return fieldComparison{order: -1, unavailable: true}
	case left < right:
		return fieldComparison{order: -1}
	case left > right:
		return fieldComparison{order: 1}
	default:
		return fieldComparison{}
	}
}
