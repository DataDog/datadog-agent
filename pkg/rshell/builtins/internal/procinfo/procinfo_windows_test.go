// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package procinfo

import (
	"context"
	"os"
	"testing"
	"time"
	"unicode/utf16"
	"unsafe"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

func TestProcessEntryToProcUsesExeNameOnly(t *testing.T) {
	var entry windows.ProcessEntry32
	entry.ProcessID = 123
	entry.ParentProcessID = 1
	for i, ch := range utf16.Encode([]rune("safeproc.exe")) {
		entry.ExeFile[i] = ch
	}

	info := processEntryToProc(&entry)

	require.Equal(t, 123, info.PID)
	require.Equal(t, "safeproc.exe", info.Cmd)
	require.NotContains(t, info.Cmd, "--token")
}

func TestGetByPIDsWithMetricsWindowsSelf(t *testing.T) {
	metrics := MetricStartTime | MetricCPUTime | MetricElapsed |
		MetricRSS | MetricVSZ | MetricPMem | MetricPCPU

	procs, err := getByPIDs(context.Background(), "", []int{os.Getpid()}, metrics)

	require.NoError(t, err)
	require.Len(t, procs, 1)
	proc := procs[0]
	require.True(t, proc.Has(metrics))
	require.Positive(t, proc.RSSKiB)
	require.Positive(t, proc.VSZKiB)
	require.GreaterOrEqual(t, proc.PMem, 0.0)
	require.GreaterOrEqual(t, proc.PCPU, 0.0)
	require.False(t, proc.StartTime.IsZero())
	require.Positive(t, proc.Elapsed)
}

func TestParseSystemProcessMemoryUsesWorkingSetAndVirtualSize(t *testing.T) {
	recordSize := int(unsafe.Sizeof(windows.SYSTEM_PROCESS_INFORMATION{}))
	buffer := make([]byte, recordSize*2)

	first := (*windows.SYSTEM_PROCESS_INFORMATION)(unsafe.Pointer(&buffer[0]))
	first.NextEntryOffset = uint32(recordSize)
	first.UniqueProcessID = 101
	first.WorkingSetSize = 12 * 1024
	first.VirtualSize = 34 * 1024
	first.PagefileUsage = 999 * 1024

	second := (*windows.SYSTEM_PROCESS_INFORMATION)(unsafe.Pointer(&buffer[recordSize]))
	second.UniqueProcessID = 202
	second.WorkingSetSize = 56 * 1024
	second.VirtualSize = 78 * 1024
	second.PagefileUsage = 888 * 1024

	metrics, ok := parseSystemProcessMemory(buffer)

	require.True(t, ok)
	require.Equal(t, windowsProcessMemory{rssKiB: 12, vszKiB: 34}, metrics[101])
	require.Equal(t, windowsProcessMemory{rssKiB: 56, vszKiB: 78}, metrics[202])
}

func TestParseSystemProcessMemoryRejectsMalformedOffset(t *testing.T) {
	recordSize := int(unsafe.Sizeof(windows.SYSTEM_PROCESS_INFORMATION{}))
	buffer := make([]byte, recordSize)
	record := (*windows.SYSTEM_PROCESS_INFORMATION)(unsafe.Pointer(&buffer[0]))
	record.NextEntryOffset = uint32(recordSize - 1)

	metrics, ok := parseSystemProcessMemory(buffer)

	require.False(t, ok)
	require.Nil(t, metrics)
}

func TestParseSystemProcessMemoryRejectsMalformedBuffers(t *testing.T) {
	recordSize := int(unsafe.Sizeof(windows.SYSTEM_PROCESS_INFORMATION{}))
	recordAlignment := int(unsafe.Alignof(windows.SYSTEM_PROCESS_INFORMATION{}))

	shortBuffer := make([]byte, recordSize-1)

	truncatedRecord := make([]byte, recordSize*2-1)
	truncatedFirst := (*windows.SYSTEM_PROCESS_INFORMATION)(unsafe.Pointer(&truncatedRecord[0]))
	truncatedFirst.NextEntryOffset = uint32(recordSize)

	misalignedOffset := recordSize + 1
	for misalignedOffset%recordAlignment == 0 {
		misalignedOffset++
	}
	misalignedRecord := make([]byte, misalignedOffset+recordSize)
	misalignedFirst := (*windows.SYSTEM_PROCESS_INFORMATION)(unsafe.Pointer(&misalignedRecord[0]))
	misalignedFirst.NextEntryOffset = uint32(misalignedOffset)

	pastEndRecord := make([]byte, recordSize)
	pastEndFirst := (*windows.SYSTEM_PROCESS_INFORMATION)(unsafe.Pointer(&pastEndRecord[0]))
	pastEndFirst.NextEntryOffset = uint32(recordSize * 2)

	tests := []struct {
		name   string
		buffer []byte
	}{
		{name: "short first record", buffer: shortBuffer},
		{name: "truncated next record", buffer: truncatedRecord},
		{name: "misaligned next record", buffer: misalignedRecord},
		{name: "next record past end", buffer: pastEndRecord},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metrics, ok := parseSystemProcessMemory(tt.buffer)

			require.False(t, ok)
			require.Nil(t, metrics)
		})
	}
}

func TestNextSystemProcessInfoSizeUsesFinalBoundedAttempt(t *testing.T) {
	next, ok := nextSystemProcessInfoSize(20<<20, 21<<20)
	require.True(t, ok)
	require.Equal(t, uint32(maxSystemProcessInfoBytes), next)

	_, ok = nextSystemProcessInfoSize(maxSystemProcessInfoBytes, maxSystemProcessInfoBytes+1)
	require.False(t, ok)
}

func TestQuerySystemProcessMemoryRetriesLengthMismatch(t *testing.T) {
	recordSize := int(unsafe.Sizeof(windows.SYSTEM_PROCESS_INFORMATION{}))
	var sizes []int

	metrics, ok := querySystemProcessMemoryWith(func(buffer []byte) (uint32, error) {
		sizes = append(sizes, len(buffer))
		if len(sizes) == 1 {
			return uint32(len(buffer) + 1), windows.STATUS_INFO_LENGTH_MISMATCH
		}
		record := (*windows.SYSTEM_PROCESS_INFORMATION)(unsafe.Pointer(&buffer[0]))
		record.UniqueProcessID = 123
		record.WorkingSetSize = 12 * 1024
		record.VirtualSize = 34 * 1024
		return uint32(recordSize), nil
	})

	require.True(t, ok)
	require.Equal(t, []int{initialSystemProcessInfoBytes, initialSystemProcessInfoBytes * 2}, sizes)
	require.Equal(t, windowsProcessMemory{rssKiB: 12, vszKiB: 34}, metrics[123])
}

func TestQuerySystemProcessMemoryRejectsInvalidResponses(t *testing.T) {
	t.Run("unexpected status", func(t *testing.T) {
		calls := 0
		metrics, ok := querySystemProcessMemoryWith(func([]byte) (uint32, error) {
			calls++
			return 0, windows.ERROR_ACCESS_DENIED
		})

		require.False(t, ok)
		require.Nil(t, metrics)
		require.Equal(t, 1, calls)
	})

	t.Run("success length exceeds buffer", func(t *testing.T) {
		metrics, ok := querySystemProcessMemoryWith(func(buffer []byte) (uint32, error) {
			return uint32(len(buffer) + 1), nil
		})

		require.False(t, ok)
		require.Nil(t, metrics)
	})

	t.Run("retry remains bounded", func(t *testing.T) {
		var sizes []int
		metrics, ok := querySystemProcessMemoryWith(func(buffer []byte) (uint32, error) {
			sizes = append(sizes, len(buffer))
			return maxSystemProcessInfoBytes + 1, windows.STATUS_INFO_LENGTH_MISMATCH
		})

		require.False(t, ok)
		require.Nil(t, metrics)
		require.NotEmpty(t, sizes)
		require.Equal(t, maxSystemProcessInfoBytes, sizes[len(sizes)-1])
		for _, size := range sizes {
			require.LessOrEqual(t, size, maxSystemProcessInfoBytes)
		}
	})
}

func TestApplyWindowsMemoryTracksUnavailablePercentage(t *testing.T) {
	var info ProcInfo
	memory := windowsProcessMemory{rssKiB: 256, vszKiB: 1024}

	applyWindowsMemory(
		&info,
		MetricRSS|MetricVSZ|MetricPMem,
		memory,
		0,
		false,
	)

	require.Equal(t, uint64(256), info.RSSKiB)
	require.Equal(t, uint64(1024), info.VSZKiB)
	require.True(t, info.Has(MetricRSS|MetricVSZ))
	require.False(t, info.Has(MetricPMem))

	applyWindowsMemory(&info, MetricPMem, memory, 1024*1024, true)

	require.InDelta(t, 25, info.PMem, 0.001)
	require.True(t, info.Has(MetricPMem))
}

func TestApplyWindowsProcessTimesUsesLifetimeAverageCPU(t *testing.T) {
	start := time.Date(2026, time.July, 24, 12, 30, 0, 0, time.Local)
	now := start.Add(10 * time.Second)
	creation := windowsFiletimeFromTicks(
		filetimeUnixEpochTicks + uint64(start.UnixNano()/100),
	)
	kernel := windowsFiletimeFromTicks(uint64((2 * time.Second) / (100 * time.Nanosecond)))
	user := windowsFiletimeFromTicks(uint64(1 * time.Second / (100 * time.Nanosecond)))
	var info ProcInfo

	applyWindowsProcessTimes(
		&info,
		MetricStartTime|MetricCPUTime|MetricElapsed|MetricPCPU,
		now,
		creation,
		kernel,
		user,
	)

	require.True(t, info.Has(MetricStartTime|MetricCPUTime|MetricElapsed|MetricPCPU))
	require.Equal(t, start.UnixNano(), info.StartTime.UnixNano())
	require.Equal(t, 3*time.Second, info.CPUTime)
	require.Equal(t, "00:00:03", info.Time)
	require.Equal(t, 10*time.Second, info.Elapsed)
	require.InDelta(t, 30, info.PCPU, 0.001)
	require.Equal(t, 30, info.CPU)
}

func TestApplyWindowsStandaloneDerivedMetrics(t *testing.T) {
	start := time.Date(2026, time.July, 24, 12, 30, 0, 0, time.Local)
	now := start.Add(10 * time.Second)
	creation := windowsFiletimeFromTicks(
		filetimeUnixEpochTicks + uint64(start.UnixNano()/100),
	)
	kernel := windowsFiletimeFromTicks(uint64((2 * time.Second) / (100 * time.Nanosecond)))
	user := windowsFiletimeFromTicks(uint64(1 * time.Second / (100 * time.Nanosecond)))
	memory := windowsProcessMemory{rssKiB: 256, vszKiB: 1024}

	tests := []struct {
		name   string
		metric Metrics
		apply  func(*ProcInfo, Metrics)
		check  func(*testing.T, ProcInfo)
	}{
		{
			name:   "pcpu",
			metric: MetricPCPU,
			apply: func(info *ProcInfo, metrics Metrics) {
				applyWindowsProcessTimes(info, metrics, now, creation, kernel, user)
			},
			check: func(t *testing.T, info ProcInfo) {
				require.InDelta(t, 30, info.PCPU, 0.001)
				require.Equal(t, 30, info.CPU)
				require.True(t, info.StartTime.IsZero())
				require.Zero(t, info.CPUTime)
				require.Zero(t, info.Elapsed)
			},
		},
		{
			name:   "pmem",
			metric: MetricPMem,
			apply: func(info *ProcInfo, metrics Metrics) {
				applyWindowsMemory(info, metrics, memory, 1024*1024, true)
			},
			check: func(t *testing.T, info ProcInfo) {
				require.InDelta(t, 25, info.PMem, 0.001)
				require.Zero(t, info.RSSKiB)
				require.Zero(t, info.VSZKiB)
			},
		},
		{
			name:   "elapsed",
			metric: MetricElapsed,
			apply: func(info *ProcInfo, metrics Metrics) {
				applyWindowsProcessTimes(info, metrics, now, creation, kernel, user)
			},
			check: func(t *testing.T, info ProcInfo) {
				require.Equal(t, 10*time.Second, info.Elapsed)
				require.True(t, info.StartTime.IsZero())
				require.Zero(t, info.CPUTime)
				require.Zero(t, info.PCPU)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := ProcInfo{PID: 123, Cmd: "safeproc", Time: "-"}

			tt.apply(&info, tt.metric)

			require.Equal(t, tt.metric, info.Available)
			require.Equal(t, 123, info.PID)
			require.Equal(t, "safeproc", info.Cmd)
			require.Equal(t, "-", info.Time)
			tt.check(t, info)
		})
	}
}

func TestApplyWindowsProcessTimesLeavesFailedDependenciesUnavailable(t *testing.T) {
	now := time.Date(2026, time.July, 24, 12, 30, 0, 0, time.Local)
	invalidCreation := windowsFiletimeFromTicks(filetimeUnixEpochTicks - 1)
	kernel := windowsFiletimeFromTicks(uint64(time.Second / (100 * time.Nanosecond)))
	info := ProcInfo{PID: 123, Cmd: "safeproc", Time: "-"}

	applyWindowsProcessTimes(
		&info,
		MetricCPUTime|MetricElapsed|MetricPCPU,
		now,
		invalidCreation,
		kernel,
		windows.Filetime{},
	)

	require.True(t, info.Has(MetricCPUTime))
	require.False(t, info.Has(MetricElapsed))
	require.False(t, info.Has(MetricPCPU))
	require.Equal(t, time.Second, info.CPUTime)
	require.Equal(t, 123, info.PID)
	require.Equal(t, "safeproc", info.Cmd)
}

func TestApplyWindowsProcessTimesRejectsInvalidElapsedInputs(t *testing.T) {
	now := time.Date(2026, time.July, 24, 12, 30, 0, 0, time.Local)
	kernel := windowsFiletimeFromTicks(uint64(time.Second / (100 * time.Nanosecond)))

	t.Run("future start", func(t *testing.T) {
		future := now.Add(time.Second)
		creation := windowsFiletimeFromTicks(
			filetimeUnixEpochTicks + uint64(future.UnixNano()/100),
		)
		info := ProcInfo{PID: 123, Cmd: "safeproc", Time: "-"}

		applyWindowsProcessTimes(
			&info,
			MetricStartTime|MetricElapsed|MetricPCPU,
			now,
			creation,
			kernel,
			windows.Filetime{},
		)

		require.Equal(t, MetricStartTime, info.Available)
		require.Equal(t, future.UnixNano(), info.StartTime.UnixNano())
		require.Zero(t, info.Elapsed)
		require.Zero(t, info.PCPU)
		require.Equal(t, 123, info.PID)
		require.Equal(t, "safeproc", info.Cmd)
	})

	t.Run("zero elapsed", func(t *testing.T) {
		creation := windowsFiletimeFromTicks(
			filetimeUnixEpochTicks + uint64(now.UnixNano()/100),
		)
		info := ProcInfo{PID: 123, Cmd: "safeproc", Time: "-"}

		applyWindowsProcessTimes(
			&info,
			MetricElapsed|MetricPCPU,
			now,
			creation,
			kernel,
			windows.Filetime{},
		)

		require.Equal(t, MetricElapsed, info.Available)
		require.Zero(t, info.Elapsed)
		require.Zero(t, info.PCPU)
		require.Equal(t, 123, info.PID)
		require.Equal(t, "safeproc", info.Cmd)
	})
}

func TestWindowsTimeConversionsRejectOverflow(t *testing.T) {
	t.Run("filetime before unix epoch", func(t *testing.T) {
		_, ok := windowsFiletimeToTime(windowsFiletimeFromTicks(filetimeUnixEpochTicks - 1))

		require.False(t, ok)
	})

	t.Run("filetime duration overflow", func(t *testing.T) {
		_, ok := windowsFiletimeToTime(
			windowsFiletimeFromTicks(filetimeUnixEpochTicks + maxDurationTicks + 1),
		)

		require.False(t, ok)
	})

	t.Run("cpu tick sum overflow", func(t *testing.T) {
		_, ok := windowsCPUTime(
			windowsFiletimeFromTicks(^uint64(0)),
			windowsFiletimeFromTicks(1),
		)

		require.False(t, ok)
	})

	t.Run("cpu duration overflow", func(t *testing.T) {
		_, ok := windowsCPUTime(
			windowsFiletimeFromTicks(maxDurationTicks+1),
			windows.Filetime{},
		)

		require.False(t, ok)
	})
}

func windowsFiletimeFromTicks(ticks uint64) windows.Filetime {
	return windows.Filetime{
		LowDateTime:  uint32(ticks),
		HighDateTime: uint32(ticks >> 32),
	}
}
