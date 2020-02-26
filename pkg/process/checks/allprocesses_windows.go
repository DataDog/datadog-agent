// +build windows

package checks

import (
	"bytes"
	"strings"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
	cpu "github.com/DataDog/gopsutil/cpu"
	process "github.com/DataDog/gopsutil/process"

	"github.com/shirou/w32"

	"golang.org/x/sys/windows"
)

var (
	modpsapi                  = windows.NewLazyDLL("psapi.dll")
	modkernel                 = windows.NewLazyDLL("kernel32.dll")
	procGetProcessMemoryInfo  = modpsapi.NewProc("GetProcessMemoryInfo")
	procGetProcessHandleCount = modkernel.NewProc("GetProcessHandleCount")
	procGetProcessIoCounters  = modkernel.NewProc("GetProcessIoCounters")

	// XXX: Cross-check state is stored globally so the checks are not thread-safe.
	cachedProcesses  = map[uint32]cachedProcess{}
	checkCount       = 0
	haveWarnedNoArgs = false
)

type SystemProcessInformation struct {
	NextEntryOffset   uint64
	NumberOfThreads   uint64
	Reserved1         [48]byte
	Reserved2         [3]byte
	UniqueProcessID   uintptr
	Reserved3         uintptr
	HandleCount       uint64
	Reserved4         [4]byte
	Reserved5         [11]byte
	PeakPagefileUsage uint64
	PrivatePageCount  uint64
	Reserved6         [6]uint64
}

type IO_COUNTERS struct {
	ReadOperationCount  uint64
	WriteOperationCount uint64
	OtherOperationCount uint64
	ReadTransferCount   uint64
	WriteTransferCount  uint64
	OtherTransferCount  uint64
}

func getProcessMemoryInfo(h windows.Handle, mem *process.PROCESS_MEMORY_COUNTERS) (err error) {
	r1, _, e1 := procGetProcessMemoryInfo.Call(uintptr(h), uintptr(unsafe.Pointer(mem)), uintptr(unsafe.Sizeof(*mem)))
	if r1 == 0 {
		return e1
	}
	return nil
}

func getProcessHandleCount(h windows.Handle, count *uint32) (err error) {
	r1, _, e1 := procGetProcessHandleCount.Call(uintptr(h), uintptr(unsafe.Pointer(count)))
	if r1 == 0 {
		return e1
	}
	return nil
}

func getProcessIoCounters(h windows.Handle, counters *IO_COUNTERS) (err error) {
	r1, _, e1 := procGetProcessIoCounters.Call(uintptr(h), uintptr(unsafe.Pointer(counters)))
	if r1 == 0 {
		return e1
	}
	return nil
}

func getAllProcesses(cfg *config.AgentConfig) (map[int32]*process.FilledProcess, error) {
	allProcsSnap := w32.CreateToolhelp32Snapshot(w32.TH32CS_SNAPPROCESS, 0)
	if allProcsSnap == 0 {
		return nil, windows.GetLastError()
	}
	procs := make(map[int32]*process.FilledProcess)

	defer w32.CloseHandle(allProcsSnap)
	var pe32 w32.PROCESSENTRY32
	pe32.DwSize = uint32(unsafe.Sizeof(pe32))

	checkCount++
	knownPids := makePidSet()

	for success := w32.Process32First(allProcsSnap, &pe32); success; success = w32.Process32Next(allProcsSnap, &pe32) {
		pid := pe32.Th32ProcessID
		ppid := pe32.Th32ParentProcessID

		if pid == 0 {
			// this is the "system idle process".  We'll never be able to open it,
			// which will cause us to thrash WMI once per check, which we don't
			// want to do.
			continue
		}
		cp, ok := cachedProcesses[pid]
		if !ok {
			// wasn't already in the map.
			cp = cachedProcess{}

			if err := cp.fillFromProcEntry(&pe32); err != nil {
				log.Debugf("could not fill Win32 process information for pid %v %v", pid, err)
				continue
			}
			cachedProcesses[pid] = cp
		}
		procHandle := cp.procHandle

		var CPU windows.Rusage
		if err := windows.GetProcessTimes(procHandle, &CPU.CreationTime, &CPU.ExitTime, &CPU.KernelTime, &CPU.UserTime); err != nil {
			log.Debugf("Could not get process times for %v %v", pid, err)
			continue
		}

		var handleCount uint32
		if err := getProcessHandleCount(procHandle, &handleCount); err != nil {
			log.Debugf("could not get handle count for %v %v", pid, err)
			continue
		}

		var pmemcounter process.PROCESS_MEMORY_COUNTERS
		if err := getProcessMemoryInfo(procHandle, &pmemcounter); err != nil {
			log.Debugf("could not get memory info for %v %v", pid, err)
			continue
		}

		// shell out to getprocessiocounters for io stats
		var ioCounters IO_COUNTERS
		if err := getProcessIoCounters(procHandle, &ioCounters); err != nil {
			log.Debugf("could not get IO Counters for %v %v", pid, err)
			continue
		}
		ctime := CPU.CreationTime.Nanoseconds() / 1000000

		utime := float64((int64(CPU.UserTime.HighDateTime) << 32) | int64(CPU.UserTime.LowDateTime))
		stime := float64((int64(CPU.KernelTime.HighDateTime) << 32) | int64(CPU.KernelTime.LowDateTime))

		delete(knownPids, pid)
		procs[int32(pid)] = &process.FilledProcess{
			Pid:     int32(pid),
			Ppid:    int32(ppid),
			Cmdline: cp.parsedArgs,
			CpuTime: cpu.TimesStat{
				User:      utime,
				System:    stime,
				Timestamp: time.Now().UnixNano(),
			},

			CreateTime:  ctime,
			OpenFdCount: int32(handleCount),
			NumThreads:  int32(pe32.CntThreads),
			CtxSwitches: &process.NumCtxSwitchesStat{},
			MemInfo: &process.MemoryInfoStat{
				RSS:  uint64(pmemcounter.WorkingSetSize),
				VMS:  uint64(pmemcounter.QuotaPagedPoolUsage),
				Swap: 0,
			},
			Exe: cp.executablePath,
			IOStat: &process.IOCountersStat{
				ReadCount:  ioCounters.ReadOperationCount,
				WriteCount: ioCounters.WriteOperationCount,
				ReadBytes:  ioCounters.ReadTransferCount,
				WriteBytes: ioCounters.WriteTransferCount,
			},
			Username: cp.userName,
		}
	}
	for pid := range knownPids {
		cp := cachedProcesses[pid]
		log.Debugf("removing process %v %v", pid, cp.executablePath)
		cp.close()
		delete(cachedProcesses, pid)
	}

	return procs, nil
}

func getUsernameForProcess(h windows.Handle) (name string, err error) {
	name = ""
	err = nil
	var t windows.Token
	err = windows.OpenProcessToken(h, windows.TOKEN_QUERY, &t)
	if err != nil {
		log.Debugf("Failed to open process token %v", err)
		return
	}
	defer t.Close()
	tokenUser, err := t.GetTokenUser()

	user, domain, _, err := tokenUser.User.Sid.LookupAccount("")
	if nil != err {
		return "", err
	}
	return domain + "\\" + user, err
}

func convertWindowsString(winput []uint16) string {
	var buf bytes.Buffer
	for _, r := range winput {
		if r == 0 {
			break
		}
		buf.WriteRune(rune(r))
	}
	return buf.String()
}

func parseCmdLineArgs(cmdline string) (res []string) {
	blocks := strings.Split(cmdline, " ")
	findCloseQuote := false
	donestring := false

	var stringInProgress bytes.Buffer
	for _, b := range blocks {
		numquotes := strings.Count(b, "\"")
		if numquotes == 0 {
			stringInProgress.WriteString(b)
			if !findCloseQuote {
				donestring = true
			} else {
				stringInProgress.WriteString(" ")
			}

		} else if numquotes == 1 {
			stringInProgress.WriteString(b)
			if findCloseQuote {
				donestring = true
			} else {
				findCloseQuote = true
				stringInProgress.WriteString(" ")
			}

		} else if numquotes == 2 {
			stringInProgress.WriteString(b)
			donestring = true
		} else {
			log.Warnf("unexpected quotes in string, giving up (%v)", cmdline)
			return res
		}

		if donestring {
			res = append(res, stringInProgress.String())
			stringInProgress.Reset()
			findCloseQuote = false
			donestring = false
		}
	}
	return res
}

func makePidSet() (pids map[uint32]bool) {
	pids = make(map[uint32]bool)
	for pid := range cachedProcesses {
		pids[pid] = true
	}
	return
}

type cachedProcess struct {
	userName       string
	executablePath string
	commandLine    string
	procHandle     windows.Handle
	parsedArgs     []string
}

func (cp *cachedProcess) fillFromProcEntry(pe32 *w32.PROCESSENTRY32) (err error) {
	// 0x1000 is PROCESS_QUERY_LIMITED_INFORMATION, but that constant isn't
	// 0x10   is PROCESS_VM_READ
	// defined in x/sys/windows
	cp.procHandle, err = windows.OpenProcess(0x1010, false, uint32(pe32.Th32ProcessID))
	if err != nil {
		log.Debugf("Couldn't open process with PROCESS_VM_READ %v %v", pe32.Th32ProcessID, err)
		cp.procHandle, err = windows.OpenProcess(0x1000, false, uint32(pe32.Th32ProcessID))
		if err != nil {
			log.Debugf("Couldn't open process %v %v", pe32.Th32ProcessID, err)
			return err
		}
	}
	var usererr error
	cp.userName, usererr = getUsernameForProcess(cp.procHandle)
	if usererr != nil {
		log.Debugf("Couldn't get process username %v %v", pe32.Th32ProcessID, err)
	}
	var cmderr error
	cp.executablePath = convertWindowsString(pe32.SzExeFile[:])
	cp.commandLine, cmderr = winutil.GetCommandLineForProcess(cp.procHandle)
	if cmderr != nil {
		log.Debugf("Error retrieving full command line %v", cmderr)
		cp.commandLine = cp.executablePath
	}

	var parsedargs []string
	if len(cp.commandLine) == 0 {
		parsedargs = append(parsedargs, cp.executablePath)
	} else {
		parsedargs = parseCmdLineArgs(cp.commandLine)
	}
	cp.parsedArgs = parsedargs
	return
}

func (cp *cachedProcess) close() {
	windows.CloseHandle(cp.procHandle)
}
