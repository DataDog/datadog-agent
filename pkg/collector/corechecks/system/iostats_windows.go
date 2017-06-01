package system

import (
	"bytes"
	"fmt"
	"regexp"
	"syscall"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	log "github.com/cihub/seelog"
)

// PDH defines copied from winPDH.

// PDH_FMT_COUNTERVALUE_DOUBLE for double values
type PDH_FMT_COUNTERVALUE_DOUBLE struct {
	CStatus     uint32
	DoubleValue float64
}

// PDH_FMT_COUNTERVALUE_DOUBLE for 64 bit integer values
type PDH_FMT_COUNTERVALUE_LARGE struct {
	CStatus    uint32
	LargeValue int64
}

// PDH_FMT_COUNTERVALUE_DOUBLE for long values
type PDH_FMT_COUNTERVALUE_LONG struct {
	CStatus   uint32
	LongValue int32
	padding   [4]byte
}

// windows system const
const (
	ERROR_SUCCESS        = 0
	ERROR_FILE_NOT_FOUND = 2
	DRIVE_REMOVABLE      = 2
	DRIVE_FIXED          = 3
	HKEY_LOCAL_MACHINE   = 0x80000002
	RRF_RT_REG_SZ        = 0x00000002
	RRF_RT_REG_DWORD     = 0x00000010
	PDH_FMT_LONG         = 0x00000100
	PDH_FMT_DOUBLE       = 0x00000200
	PDH_FMT_LARGE        = 0x00000400
	PDH_INVALID_DATA     = 0xc0000bc6
	PDH_INVALID_HANDLE   = 0xC0000bbc
	PDH_NO_DATA          = 0x800007d5
)

var (
	Modkernel32 = syscall.NewLazyDLL("kernel32.dll")
	ModNt       = syscall.NewLazyDLL("ntdll.dll")
	ModPdh      = syscall.NewLazyDLL("pdh.dll")

	ProcGetSystemTimes           = Modkernel32.NewProc("GetSystemTimes")
	ProcNtQuerySystemInformation = ModNt.NewProc("NtQuerySystemInformation")
	ProcGetLogicalDriveStringsW  = Modkernel32.NewProc("GetLogicalDriveStringsW")
	ProcGetDriveType             = Modkernel32.NewProc("GetDriveTypeW")
	PdhOpenQuery                 = ModPdh.NewProc("PdhOpenQuery")
	PdhAddCounter                = ModPdh.NewProc("PdhAddCounterW")
	PdhCollectQueryData          = ModPdh.NewProc("PdhCollectQueryData")
	PdhGetFormattedCounterValue  = ModPdh.NewProc("PdhGetFormattedCounterValue")
	PdhCloseQuery                = ModPdh.NewProc("PdhCloseQuery")
)

type FILETIME struct {
	DwLowDateTime  uint32
	DwHighDateTime uint32
}

// borrowed from net/interface_windows.go
func BytePtrToString(p *uint8) string {
	a := (*[10000]uint8)(unsafe.Pointer(p))
	i := 0
	for a[i] != 0 {
		i++
	}
	return string(a[:i])
}

// CounterInfo
// copied from https://github.com/mackerelio/mackerel-agent/
type CounterInfo struct {
	PostName    string
	CounterName string
	Counter     syscall.Handle
}

// CreateQuery XXX
// copied from https://github.com/mackerelio/mackerel-agent/
func CreateQuery() (syscall.Handle, error) {
	var query syscall.Handle
	r, _, err := PdhOpenQuery.Call(0, 0, uintptr(unsafe.Pointer(&query)))
	if r != 0 {
		return 0, err
	}
	return query, nil
}

// CreateCounter XXX
func CreateCounter(query syscall.Handle, pname, cname string) (*CounterInfo, error) {
	var counter syscall.Handle
	r, _, err := PdhAddCounter.Call(
		uintptr(query),
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(cname))),
		0,
		uintptr(unsafe.Pointer(&counter)))
	if r != 0 {
		return nil, err
	}
	return &CounterInfo{
		PostName:    pname,
		CounterName: cname,
		Counter:     counter,
	}, nil
}

const (
	// SectorSize is exported in github.com/shirou/gopsutil/disk (but not working!)
	SectorSize = 512
	kB         = (1 << 10)
)

// IOCheck doesn't need additional fields
type IOCheck struct {
	sender    aggregator.Sender
	blacklist *regexp.Regexp
	query     syscall.Handle
	drivemap  map[string][]*CounterInfo
}

func init() {
	core.RegisterCheck("io", ioFactory)
}

var metrics = map[string]string{
	"system.io.wkb_s":    "Disk Write Bytes/sec",
	"system.io.w_s":      "Disk Writes/sec",
	"system.io.rkb_s":    "Disk Read Bytes/sec",
	"system.io.r_s":      "Disk Reads/sec",
	"system.io.avg_q_sz": "Current Disk Queue Length",
}

func ioFactory() check.Check {
	log.Infof("IOCheck factory")
	c := &IOCheck{}
	q, err := CreateQuery()
	if err != nil {
		log.Errorf("IO factory failed to create query")
		return nil
	}
	c.query = q
	c.drivemap = make(map[string][]*CounterInfo, 0)

	drivebuf := make([]byte, 256)

	r, _, err := ProcGetLogicalDriveStringsW.Call(
		uintptr(len(drivebuf)),
		uintptr(unsafe.Pointer(&drivebuf[0])))
	if r == 0 {
		log.Errorf("IO Factory failed to get drive strings")
		return nil
	}
	for _, v := range drivebuf {
		// between 'A' & 'Z'
		if v >= 65 && v <= 90 {
			drive := string(v)
			r, _, err = ProcGetDriveType.Call(uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(drive + `:\`))))
			if r != DRIVE_FIXED {
				continue
			}
			c.drivemap[drive] = make([]*CounterInfo, 0, len(metrics))
			for k, m := range metrics {
				var counter *CounterInfo
				countername := fmt.Sprintf(`\PhysicalDisk(0 %s:)\%s`, drive, m)
				counter, err = CreateCounter(c.query, k, countername)
				if err != nil {
					log.Errorf("Failed to create counter %s %d", countername, err)
					continue
				} else {
					log.Infof("Created counter name %s", countername)
				}
				c.drivemap[drive] = append(c.drivemap[drive], counter)
			}
		}
	}
	log.Infof("IO Factory -- success")
	return c
}

// Run executes the check
func (c *IOCheck) Run() error {
	log.Infof("Running IO Check")
	var err error

	r, _, err := PdhCollectQueryData.Call(uintptr(c.query))
	if r != 0 && err != nil {
		return err
	}
	time.Sleep(time.Second)
	r, _, err = PdhCollectQueryData.Call(uintptr(c.query))
	if r != 0 && err != nil {
		return err
	}
	var tagbuff bytes.Buffer
	for drive, counters := range c.drivemap {
		log.Infof("checking drive %s against blacklist", drive)
		if c.blacklist != nil && c.blacklist.MatchString(drive) {
			continue
		}

		tagbuff.Reset()
		tagbuff.WriteString("device:")
		tagbuff.WriteString(drive)
		tags := []string{tagbuff.String()}
		log.Infof("counters map is size %d", len(counters))
		for _, v := range counters {
			log.Infof("Checking counter %s", v.PostName)
			var fmtValue PDH_FMT_COUNTERVALUE_LARGE
			r, _, err := PdhGetFormattedCounterValue.Call(uintptr(v.Counter),
				PDH_FMT_LARGE,
				uintptr(0),
				uintptr(unsafe.Pointer(&fmtValue)))
			if r != 0 && err != nil {
				log.Warnf("GetFormattedCounterValue %d %d", r, err)
				return err
			}
			val := fmtValue.LargeValue
			if v.PostName == "system.io.wkb_s" ||
				v.PostName == "system.io.rkb_s" {
				val = val / 1024
			}
			log.Infof("Setting Gauge %s %f %s", v.PostName, float64(val), tags)
			c.sender.Gauge(v.PostName, float64(val), "", tags)
		}
	}
	c.sender.Commit()
	return nil
}
