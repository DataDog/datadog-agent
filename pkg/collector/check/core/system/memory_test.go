package system

import (
	"testing"

	"github.com/shirou/gopsutil/mem"
)

func VirtualMemory() (*mem.VirtualMemoryStat, error) {
	return &mem.VirtualMemoryStat{
		Total:        12345667890,
		Available:    234567890,
		Used:         10000000000,
		UsedPercent:  81,
		Free:         11554304000,
		Active:       2506516070400,
		Inactive:     970587111424,
		Wired:        0,
		Buffers:      353818902528,
		Cached:       2596446142464,
		Writeback:    0,
		Dirty:        314572800,
		WritebackTmp: 0,
		Shared:       327680000000,
		Slab:         327680000000,
		PageTables:   37790679040,
		SwapCached:   25000000000,
	}, nil
}

func SwapMemory() (*mem.SwapMemoryStat, error) {
	return &mem.SwapMemoryStat{
		Total:       100000,
		Used:        40000,
		Free:        60000,
		UsedPercent: 40,
		Sin:         21,
		Sout:        22,
	}, nil
}

func TestMemoryCheckLinux(t *testing.T) {
	virtualMemory = VirtualMemory
	swapMemory = SwapMemory
	memCheck := new(MemoryCheck)

	mock := new(MockSender) // from common_test.go
	memCheck.sender = mock
	runtimeOS = "linux"

	mock.On("Gauge", "system.mem.total", 12345667890.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.free", 11554304000.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.used", 10000000000.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.usable", 234567890.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.pct_usable", 0.19, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.cached", 2596446142464.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.shared", 327680000000.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.slab", 327680000000.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.page_tables", 37790679040.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.swap.total", 100000.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.swap.free", 60000.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.swap.used", 40000.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.swap.pct_free", 0.6, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.swap.cached", 25000000000.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Commit").Return().Times(1)
	memCheck.Run()

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", 14)
	mock.AssertNumberOfCalls(t, "Commit", 1)
}

func TestMemoryCheckFreebsd(t *testing.T) {
	virtualMemory = VirtualMemory
	swapMemory = SwapMemory
	memCheck := new(MemoryCheck)

	mock := new(MockSender)
	memCheck.sender = mock
	runtimeOS = "freebsd"

	mock.On("Gauge", "system.mem.total", 12345667890.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.free", 11554304000.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.used", 10000000000.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.usable", 234567890.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.pct_usable", 0.19, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.cached", 2596446142464.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.swap.total", 100000.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.swap.free", 60000.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.swap.used", 40000.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.swap.pct_free", 0.6, "", []string(nil)).Return().Times(1)
	mock.On("Commit").Return().Times(1)
	memCheck.Run()

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", 10)
	mock.AssertNumberOfCalls(t, "Commit", 1)
}

func TestMemoryCheckDarwin(t *testing.T) {
	virtualMemory = VirtualMemory
	swapMemory = SwapMemory
	memCheck := new(MemoryCheck)

	mock := new(MockSender)
	memCheck.sender = mock
	runtimeOS = "darwin"

	mock.On("Gauge", "system.mem.total", 12345667890.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.free", 11554304000.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.used", 10000000000.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.usable", 234567890.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.mem.pct_usable", 0.19, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.swap.total", 100000.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.swap.free", 60000.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.swap.used", 40000.0/mbSize, "", []string(nil)).Return().Times(1)
	mock.On("Gauge", "system.swap.pct_free", 0.6, "", []string(nil)).Return().Times(1)
	mock.On("Commit").Return().Times(1)
	memCheck.Run()

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", 9)
	mock.AssertNumberOfCalls(t, "Commit", 1)
}
