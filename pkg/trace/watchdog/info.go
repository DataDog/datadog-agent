package watchdog

import (
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// cacheDelay should be long enough so that we don't poll the info
	// too often and waste resources doing it, and also long enough
	// so that it's not jittering (CPU can be volatile).
	// OTOH it should be short enough to get up-to-date recent info.
	cacheDelay = 20 * time.Second
)

// CPUInfo contains basic CPU info
type CPUInfo struct {
	// UserAvg is the average of the user CPU usage since last time
	// it was polled. 0 means "not used at all" and 1 means "1 CPU was
	// totally full for that period". So it might be greater than 1 if
	// the process is monopolizing several cores.
	UserAvg float64
}

// MemInfo contains basic memory info
type MemInfo struct {
	// Alloc is the number of bytes allocated and not yet freed
	// as described in runtime.MemStats.Alloc
	Alloc uint64
	// AllocPerSec is the average number of bytes allocated, per second,
	// since last time this function was called.
	AllocPerSec float64
}

// NetInfo contains basic networking info
type NetInfo struct {
	// Connections is the number of connections opened by this process.
	Connections int32
}

// Info contains all the watchdog infos, to be published by expvar
type Info struct {
	// CPU contains basic CPU info
	CPU CPUInfo
	// Mem contains basic Mem info
	Mem MemInfo
	// Net contains basic Net info
	Net NetInfo
}

// CurrentInfo is used to query CPU and Mem info, it keeps data from
// the previous calls to calculate averages. It is not thread safe.
type CurrentInfo struct {
	pid        int32
	mu         sync.Mutex
	cacheDelay time.Duration

	lastCPUTime time.Time
	lastCPUUser float64
	lastCPU     CPUInfo

	lastMemTime       time.Time
	lastMemTotalAlloc uint64
	lastMem           MemInfo

	lastNetTime time.Time
	lastNet     NetInfo
}

// globalCurrentInfo is a global default object one can safely use
// if only one goroutine is polling for CPU() and Mem()
var globalCurrentInfo *CurrentInfo

func init() {
	var err error
	globalCurrentInfo, err = NewCurrentInfo()
	if err != nil {
		log.Errorf("unable to create global Process: %v", err)
	}
}

// NewCurrentInfo creates a new CurrentInfo referring to the current running program.
func NewCurrentInfo() (*CurrentInfo, error) {
	return &CurrentInfo{
		pid:        int32(os.Getpid()),
		cacheDelay: cacheDelay,
	}, nil
}

// CPU returns basic CPU info.
func (pi *CurrentInfo) CPU() CPUInfo {
	pi.mu.Lock()
	defer pi.mu.Unlock()

	now := time.Now()
	dt := now.Sub(pi.lastCPUTime)
	if dt <= pi.cacheDelay {
		return pi.lastCPU // don't query too often, cache a little bit
	}
	pi.lastCPUTime = now

	userTime, err := cpuTimeUser(pi.pid)
	if err != nil {
		log.Debugf("unable to get CPU times: %v", err)
		return pi.lastCPU
	}

	dua := userTime - pi.lastCPUUser
	pi.lastCPUUser = userTime
	if dua <= 0 {
		pi.lastCPU.UserAvg = 0 // shouldn't happen, but make sure result is always > 0
	} else {
		pi.lastCPU.UserAvg = float64(time.Second) * dua / float64(dt)
		pi.lastCPUUser = userTime
	}

	return pi.lastCPU
}

// Mem returns basic memory info.
func (pi *CurrentInfo) Mem() MemInfo {
	pi.mu.Lock()
	defer pi.mu.Unlock()

	now := time.Now()
	dt := now.Sub(pi.lastMemTime)
	if dt <= pi.cacheDelay {
		return pi.lastMem // don't query too often, cache a little bit
	}
	pi.lastMemTime = now

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	ret := MemInfo{Alloc: ms.Alloc, AllocPerSec: pi.lastMem.AllocPerSec}

	dta := int64(ms.TotalAlloc) - int64(pi.lastMemTotalAlloc)
	pi.lastMemTotalAlloc = ms.TotalAlloc
	if dta <= 0 {
		pi.lastMem.AllocPerSec = 0 // shouldn't happen, but make sure result is always > 0
	} else {
		pi.lastMem.AllocPerSec = float64(time.Second) * float64(dta) / float64(dt)
	}
	ret.AllocPerSec = pi.lastMem.AllocPerSec

	return ret
}

// CPU returns basic CPU info.
func CPU() CPUInfo {
	if globalCurrentInfo == nil {
		return CPUInfo{}
	}
	return globalCurrentInfo.CPU()
}

// Mem returns basic memory info.
func Mem() MemInfo {
	if globalCurrentInfo == nil {
		return MemInfo{}
	}
	return globalCurrentInfo.Mem()
}

// Net returns basic network info.
func Net() NetInfo {
	if globalCurrentInfo == nil {
		return NetInfo{}
	}
	return globalCurrentInfo.Net()
}
