// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ebpf

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/DataDog/ebpf-manager/tracefs"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
)

type checkMap struct {
	name      string
	mtype     ebpf.MapType
	lockCount uint32
	alloc     func(*ebpf.MapSpec) *ebpf.Map
}

var specs map[ebpf.MapType]ebpf.MapSpec = map[ebpf.MapType]ebpf.MapSpec{
	ebpf.Hash: {
		Name:       "test_hash",
		Type:       ebpf.Hash,
		KeySize:    4,
		ValueSize:  4,
		MaxEntries: 1,
	},
	ebpf.PerCPUHash: {
		Name:       "test_percpu_hash",
		Type:       ebpf.PerCPUHash,
		KeySize:    4,
		ValueSize:  4,
		MaxEntries: 1,
	},
	ebpf.LRUHash: {
		Name:       "test_lru",
		Type:       ebpf.LRUHash,
		KeySize:    4,
		ValueSize:  4,
		MaxEntries: 1,
	},
	ebpf.LRUCPUHash: {
		Name:       "test_pcpu_lru",
		Type:       ebpf.LRUCPUHash,
		KeySize:    4,
		ValueSize:  4,
		MaxEntries: 1,
	},
	ebpf.RingBuf: {
		Name:       "test_ringbuf",
		Type:       ebpf.RingBuf,
		MaxEntries: 4096,
	},
}

func entries(mp *ebpf.Map) uint32 {
	iter := mp.Iterate()

	var val uint32
	var key LockRange

	var count uint32
	for iter.Next(&key, &val) {
		count++
	}

	return count
}

func lruLockCount(cpu uint32) uint32 {
	return hashMapLockRanges(cpu) + lruMapLockRanges(cpu)
}

func pcpuLruLockCount(cpu uint32) uint32 {
	return hashMapLockRanges(cpu) + pcpuLruMapLockRanges(cpu)
}

func TestLockRanges(t *testing.T) {
	if !lockContentionCollectorSupported() {
		t.Skip("EBPF lock contention collector not supported")
	}

	cpu, err := kernel.PossibleCPUs()
	require.NoError(t, err)

	cases := []checkMap{
		{
			name:      "Hashmap",
			mtype:     ebpf.Hash,
			lockCount: hashMapLockRanges(uint32(cpu)),
			alloc: func(spec *ebpf.MapSpec) *ebpf.Map {
				m, err := ebpf.NewMap(spec)
				require.NoError(t, err)
				return m
			},
		},
		{
			name:      "Percpu-Hashmap",
			mtype:     ebpf.PerCPUHash,
			lockCount: hashMapLockRanges(uint32(cpu)),
			alloc: func(spec *ebpf.MapSpec) *ebpf.Map {
				m, err := ebpf.NewMap(spec)
				require.NoError(t, err)
				return m
			},
		},
		{
			name:      "LRUHash",
			mtype:     ebpf.LRUHash,
			lockCount: lruLockCount(uint32(cpu)),
			alloc: func(spec *ebpf.MapSpec) *ebpf.Map {
				m, err := ebpf.NewMap(spec)
				require.NoError(t, err)
				return m
			},
		},
		{
			name:      "LRUPcpuHash",
			mtype:     ebpf.LRUCPUHash,
			lockCount: pcpuLruLockCount(uint32(cpu)),
			alloc: func(spec *ebpf.MapSpec) *ebpf.Map {
				m, err := ebpf.NewMap(spec)
				require.NoError(t, err)
				return m
			},
		},
		{
			name:      "RingBuf",
			mtype:     ebpf.RingBuf,
			lockCount: ringbufMapLockRanges(uint32(cpu)),
			alloc: func(spec *ebpf.MapSpec) *ebpf.Map {
				m, err := ebpf.NewMap(spec)
				require.NoError(t, err)
				return m
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			l := NewLockContentionCollector()
			require.NotNil(t, l)

			spec := specs[c.mtype]
			m := c.alloc(&spec)

			t.Cleanup(func() {
				m.Close()
				l.Close()
				ResetAllMappings()
			})

			mInfo, err := m.Info()
			require.NoError(t, err)

			id, _ := mInfo.ID()
			mapNameMapping[uint32(id)] = spec.Name

			err = l.Initialize(false)
			require.NoError(t, err)

			require.Equal(t, entries(l.objects.MapAddrFd), c.lockCount)
		})
	}
}

func TestLoadWithMaxTrackedRanges(t *testing.T) {
	if !lockContentionCollectorSupported() {
		t.Skip("EBPF lock contention collector not supported")
	}
	flake.MarkOnJobName(t, "ubuntu_25.10")

	l := NewLockContentionCollector()
	require.NotNil(t, l)

	staticRanges = true
	err := l.Initialize(true)
	require.NoError(t, err)

	l.Close()
}

// Values returned by get_nesting_depth() in pkg/ebpf/c/preempt.h.
const (
	taskDepth    int32 = 0
	softirqDepth int32 = 1
	hardirqDepth int32 = 2
	nmiDepth     int32 = 3

	// noSample is written to the nesting_depth map before triggering, so a
	// value left over from an earlier subtest cannot be read as a fresh sample.
	noSample int32 = -99
)

func tracepointExists(group, name string) bool {
	root, err := tracefs.Root()
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(root, "events", group, name, "id"))
	return err == nil
}

// loadPreemptTest loads preempt_test.o and attaches only progName. All three
// programs in that object share one single-entry map, so attaching just the
// program under test is what makes the observed value attributable to the
// context being exercised.
func loadPreemptTest(t *testing.T, progName, tracepoint string) *ebpf.Map {
	var depths *ebpf.Map

	err := LoadCOREAsset("preempt_test.o", func(bc bytecode.AssetReader, opts manager.Options) error {
		spec, err := ebpf.LoadCollectionSpecFromReader(bc)
		require.NoError(t, err)

		// Mirror LockContentionCollector.Initialize: read the per-cpu
		// __preempt_count symbol when the kernel exports it, otherwise fall
		// back to pcpu_hot, for x86.
		if v, ok := spec.Variables["use_preempt_count"]; ok {
			missing, err := VerifyKernelFuncs("__preempt_count")
			require.NoError(t, err)
			if len(missing) == 0 {
				require.NoError(t, v.Set(uint64(1)))
			}
		}

		// Load only the program under test. Some of the programs in this
		// object are arch specific -- local_timer_entry exists on x86 only,
		// irq_handler_entry is its arm64 counterpart -- so loading the whole
		// collection would pull in programs that cannot attach here.
		for name := range spec.Programs {
			if name != progName {
				delete(spec.Programs, name)
			}
		}

		coll, err := ebpf.NewCollectionWithOptions(spec, ebpf.CollectionOptions{
			Programs: ebpf.ProgramOptions{
				KernelTypes: opts.VerifierOptions.Programs.KernelTypes,
			},
		})
		var ve *ebpf.VerifierError
		if errors.As(err, &ve) {
			t.Fatalf("verifier error loading preempt_test.o: %+v", ve)
		}
		require.NoError(t, err)
		t.Cleanup(coll.Close)

		prog, ok := coll.Programs[progName]
		require.True(t, ok, "program %s missing from preempt_test.o", progName)

		lnk, err := link.AttachRawTracepoint(link.RawTracepointOptions{
			Name:    tracepoint,
			Program: prog,
		})
		require.NoError(t, err, "attaching raw tracepoint %s", tracepoint)
		t.Cleanup(func() { _ = lnk.Close() })

		depths, ok = coll.Maps["nesting_depth"]
		require.True(t, ok, "nesting_depth map missing from preempt_test.o")

		return nil
	})
	require.NoError(t, err)

	return depths
}

func writeNestingDepth(t *testing.T, m *ebpf.Map, val int32) {
	key := uint32(0)
	require.NoError(t, m.Put(&key, &val))
}

func readNestingDepth(t *testing.T, m *ebpf.Map) int32 {
	key := uint32(0)
	var val int32
	require.NoError(t, m.Lookup(&key, &val))
	return val
}

// awaitDepth runs trigger until the attached program records a sample, then
// checks that sample reports want. Only the program for the context under test
// is attached, so every sample it takes must classify as want; the retry loop
// is there purely to wait for the trigger to land, which is asynchronous for
// device interrupts and NMI IPIs.
func awaitDepth(t *testing.T, m *ebpf.Map, want int32, trigger func()) {
	t.Helper()

	writeNestingDepth(t, m, noSample)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		trigger()

		got := readNestingDepth(t, m)
		if got == noSample {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		require.Equal(t, want, got)
		return
	}

	t.Fatal("tracepoint never fired, nesting depth was never recorded")
}

// enableSysrq makes sure sysrq commands are accepted, restoring the previous
// setting afterwards. Skips the test when sysrq is unavailable.
func enableSysrq(t *testing.T) {
	const sysrqCfg = "/proc/sys/kernel/sysrq"

	if _, err := os.Stat("/proc/sysrq-trigger"); err != nil {
		t.Skipf("/proc/sysrq-trigger unavailable: %v", err)
	}

	prev, err := os.ReadFile(sysrqCfg)
	if err != nil {
		t.Skipf("cannot read %s: %v", sysrqCfg, err)
	}
	mask, err := strconv.Atoi(strings.TrimSpace(string(prev)))
	if err != nil {
		t.Skipf("cannot parse %s: %v", sysrqCfg, err)
	}

	// Bit 3 (SYSRQ_ENABLE_DUMP) covers the 'l' command; 1 enables everything.
	if mask == 1 || mask&0x8 != 0 {
		return
	}
	if err := os.WriteFile(sysrqCfg, []byte("1"), 0o644); err != nil {
		t.Skipf("cannot enable sysrq: %v", err)
	}
	t.Cleanup(func() { _ = os.WriteFile(sysrqCfg, prev, 0o644) })
}

func TestPreemptNestingDepth(t *testing.T) {
	t.Run("Task", func(t *testing.T) {
		if !tracepointExists("raw_syscalls", "sys_enter") {
			t.Skip("raw_syscalls:sys_enter tracepoint not available")
		}

		depths := loadPreemptTest(t, "raw_tracepoint__sys_enter", "sys_enter")

		// Any syscall will do; syscalls only ever arrive from task context.
		awaitDepth(t, depths, taskDepth, func() {
			_ = syscall.Getpid()
		})
	})

	t.Run("Softirq", func(t *testing.T) {
		if !tracepointExists("irq", "softirq_entry") {
			t.Skip("irq:softirq_entry tracepoint not available")
		}

		depths := loadPreemptTest(t, "raw_tracepoint__softirq_entry", "softirq_entry")

		// A loopback send reaches netif_rx, which raises NET_RX_SOFTIRQ.
		conn, err := net.ListenPacket("udp", "127.0.0.1:0")
		require.NoError(t, err)
		t.Cleanup(func() { _ = conn.Close() })

		payload := []byte("preempt")
		awaitDepth(t, depths, softirqDepth, func() {
			for i := 0; i < 16; i++ {
				_, _ = conn.WriteTo(payload, conn.LocalAddr())
			}
		})
	})

	t.Run("Hardirq", func(t *testing.T) {
		// The timer tick drives this on both arches, but it surfaces
		// differently. x86 delivers the local APIC timer as a sysvec with its
		// own tracepoint. On arm64 the architected timer is an ordinary
		// per-cpu interrupt, so it arrives through the generic irq path.
		var group, tracepoint, prog string
		switch runtime.GOARCH {
		case "amd64":
			group, tracepoint, prog = "irq_vectors", "local_timer_entry", "raw_tracepoint__local_timer_entry"
		case "arm64":
			group, tracepoint, prog = "irq", "irq_handler_entry", "raw_tracepoint__irq_handler_entry"
		default:
			t.Skipf("preempt count is only readable on x86 and arm64, not %s", runtime.GOARCH)
		}

		if !tracepointExists(group, tracepoint) {
			t.Skipf("%s:%s tracepoint not available", group, tracepoint)
		}

		depths := loadPreemptTest(t, prog, tracepoint)

		awaitDepth(t, depths, hardirqDepth, func() {
			for start := time.Now(); time.Since(start) < 5*time.Millisecond; {
			}
		})
	})

	t.Run("NMI", func(t *testing.T) {
		if runtime.GOARCH != "amd64" {
			t.Skip("nmi:nmi_handler is only defined on x86")
		}
		if !tracepointExists("nmi", "nmi_handler") {
			t.Skip("nmi:nmi_handler tracepoint not available")
		}
		enableSysrq(t)

		depths := loadPreemptTest(t, "raw_tracepoint__nmi_handler", "nmi_handler")

		// sysrq-l runs nmi_trigger_cpumask_backtrace, which sends an NMI IPI to
		// every CPU. Each of those runs nmi_handle, where trace_nmi_handler
		// lives. This dumps backtraces to the kernel log as a side effect.
		awaitDepth(t, depths, nmiDepth, func() {
			_ = os.WriteFile("/proc/sysrq-trigger", []byte("l"), 0o200)
		})
	})
}
