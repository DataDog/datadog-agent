package cpu

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

var getCpuInfo = GetCpuInfo

// Values that need to be multiplied by the number of physical processors
var perPhysicalProcValues = []string{
	"cpu_cores",
	"cpu_logical_processors",
}

const ERROR_INSUFFICIENT_BUFFER syscall.Errno = 122
const registryHive = "HARDWARE\\DESCRIPTION\\System\\CentralProcessor\\0"

type CACHE_DESCRIPTOR struct {
	Level         uint8
	Associativity uint8
	LineSize      uint16
	Size          uint32
	cacheType     uint32
}
type SYSTEM_LOGICAL_PROCESSOR_INFORMATION struct {
	ProcessorMask uintptr
	Relationship  int // enum (int)
	// in the Windows header, this is a union of a byte, a DWORD,
	// and a CACHE_DESCRIPTOR structure
	dataunion [16]byte
}

//const SYSTEM_LOGICAL_PROCESSOR_INFORMATION_SIZE = 32

const RelationProcessorCore = 0
const RelationNumaNode = 1
const RelationCache = 2
const RelationProcessorPackage = 3
const RelationGroup = 4

type SYSTEM_INFO struct {
	wProcessorArchitecture  uint16
	wReserved               uint16
	dwPageSize              uint32
	lpMinApplicationAddress *uint32
	lpMaxApplicationAddress *uint32
	dwActiveProcessorMask   uintptr
	dwNumberOfProcessors    uint32
	dwProcessorType         uint32
	dwAllocationGranularity uint32
	wProcessorLevel         uint16
	wProcessorRevision      uint16
}

func countBits(num uint64) (count int) {
	count = 0
	for num > 0 {
		if (num & 0x1) == 1 {
			count++
		}
		num >>= 1
	}
	return
}

func getSystemInfo() (si SYSTEM_INFO) {
	var mod = syscall.NewLazyDLL("kernel32.dll")
	var gsi = mod.NewProc("GetSystemInfo")

	gsi.Call(uintptr(unsafe.Pointer(&si)))
	return
}

func computeCoresAndProcessors() (phys int, cores int, processors int, err error) {
	var mod = syscall.NewLazyDLL("kernel32.dll")
	var getProcInfo = mod.NewProc("GetLogicalProcessorInformation")
	var buflen uint32 = 0
	err = syscall.Errno(0)
	// first, figure out how much we need
	status, _, err := getProcInfo.Call(uintptr(0),
		uintptr(unsafe.Pointer(&buflen)))
	if status == 0 {
		if err != ERROR_INSUFFICIENT_BUFFER {
			// only error we're expecing here is insufficient buffer
			// anything else is a failure
			return
		}
	} else {
		// this shouldn't happen. Errno won't be set (because the fuction)
		// succeeded.  So just return something to indicate we've failed
		return 0, 0, 0, syscall.Errno(1)
	}
	buf := make([]byte, buflen)
	status, _, err = getProcInfo.Call(uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&buflen)))
	if status == 0 {
		return
	}
	// walk through each of the buffers
	var numaNodeCount int32

	for i := 0; uint32(i) < buflen; i += getSystemLogicalProcessorInformationSize() {
		info := byteArrayToProcessorStruct(buf[i : i+getSystemLogicalProcessorInformationSize()])

		switch info.Relationship {
		case RelationNumaNode:
			numaNodeCount++

		case RelationProcessorCore:
			cores++
			processors += countBits(uint64(info.ProcessorMask))

		case RelationProcessorPackage:
			phys++
		}
	}
	return
}

// GetCpuInfo returns map of interesting bits of information about the CPU
func GetCpuInfo() (cpuInfo map[string]string, err error) {

	cpuInfo = make(map[string]string)

	_, cores, lprocs, _ := computeCoresAndProcessors()
	si := getSystemInfo()

	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		registryHive,
		registry.QUERY_VALUE)
	defer k.Close()
	dw, _, err := k.GetIntegerValue("~MHz")
	cpuInfo["mhz"] = strconv.Itoa(int(dw))

	s, _, err := k.GetStringValue("ProcessorNameString")
	cpuInfo["model_name"] = s

	cpuInfo["cpu_cores"] = strconv.Itoa(cores)
	cpuInfo["cpu_logical_processors"] = strconv.Itoa(lprocs)

	s, _, err = k.GetStringValue("VendorIdentifier")
	cpuInfo["vendor_id"] = s

	s, _, err = k.GetStringValue("Identifier")
	cpuInfo["family"] = extract(s, "Family")

	cpuInfo["model"] = strconv.Itoa(int((si.wProcessorRevision >> 8) & 0xFF))
	cpuInfo["stepping"] = strconv.Itoa(int(si.wProcessorRevision & 0xFF))

	return
}

func extract(caption, field string) string {
	re := regexp.MustCompile(fmt.Sprintf("%s [0-9]* ", field))
	return strings.Split(re.FindStringSubmatch(caption)[0], " ")[1]
}
