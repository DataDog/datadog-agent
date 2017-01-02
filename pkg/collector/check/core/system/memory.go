package system

import (
	"runtime"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/check/core"
	log "github.com/cihub/seelog"
	"github.com/shirou/gopsutil/mem"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
)

// For testing purpose
var virtualMemory = mem.VirtualMemory
var swapMemory = mem.SwapMemory
var runtimeOS = runtime.GOOS

// MemoryCheck doesn't need additional fields
type MemoryCheck struct {
	sender aggregator.Sender
}

func (c *MemoryCheck) String() string {
	return "MemoryCheck"
}

const mbSize float64 = 1024 * 1024

func (c *MemoryCheck) linuxSpecificMemoryCheck(v *mem.VirtualMemoryStat, s *mem.SwapMemoryStat) {
	c.sender.Gauge("system.mem.cached", float64(v.Cached)/mbSize, "", nil)
	c.sender.Gauge("system.mem.shared", float64(v.Shared)/mbSize, "", nil)
	c.sender.Gauge("system.mem.slab", float64(v.Slab)/mbSize, "", nil)
	c.sender.Gauge("system.mem.page_tables", float64(v.PageTables)/mbSize, "", nil)
	c.sender.Gauge("system.swap.cached", float64(v.SwapCached)/mbSize, "", nil)
}

func (c *MemoryCheck) freebsdSpecificMemoryCheck(v *mem.VirtualMemoryStat, s *mem.SwapMemoryStat) {
	c.sender.Gauge("system.mem.cached", float64(v.Cached)/mbSize, "", nil)
}

// Run executes the check
func (c *MemoryCheck) Run() error {

	v, err := virtualMemory()
	if err != nil {
		log.Errorf("system.MemoryCheck: could not retrieve virtual memory stats: %s", err)
		return err
	}
	s, err := swapMemory()
	if err != nil {
		log.Errorf("system.MemoryCheck: could not retrieve swap memory stats: %s", err)
		return err
	}

	c.sender.Gauge("system.mem.total", float64(v.Total)/mbSize, "", nil)
	c.sender.Gauge("system.mem.free", float64(v.Free)/mbSize, "", nil)
	c.sender.Gauge("system.mem.used", float64(v.Used)/mbSize, "", nil)
	c.sender.Gauge("system.mem.usable", float64(v.Available)/mbSize, "", nil)
	c.sender.Gauge("system.mem.pct_usable", float64(100-v.UsedPercent)/100, "", nil)

	c.sender.Gauge("system.swap.total", float64(s.Total)/mbSize, "", nil)
	c.sender.Gauge("system.swap.free", float64(s.Free)/mbSize, "", nil)
	c.sender.Gauge("system.swap.used", float64(s.Used)/mbSize, "", nil)
	c.sender.Gauge("system.swap.pct_free", float64(100-s.UsedPercent)/100, "", nil)

	switch runtimeOS {
	case "linux":
		c.linuxSpecificMemoryCheck(v, s)
	case "freebsd":
		c.freebsdSpecificMemoryCheck(v, s)
	}

	c.sender.Commit()
	return nil
}

// Configure the Python check from YAML data
func (c *MemoryCheck) Configure(data check.ConfigData) error {
	// do nothing
	return nil
}

// InitSender initializes a sender
func (c *MemoryCheck) InitSender() {
	s, err := aggregator.GetSender()
	if err != nil {
		log.Error(err)
		return
	}

	c.sender = s
}

// Interval returns the scheduling time for the check
func (c *MemoryCheck) Interval() time.Duration {
	return check.DefaultCheckInterval
}

// ID FIXME: this should return a real identifier
func (c *MemoryCheck) ID() string {
	return c.String()
}

// Stop does nothing
func (c *MemoryCheck) Stop() {}

func init() {
	core.RegisterCheck("memory", &MemoryCheck{})
}
