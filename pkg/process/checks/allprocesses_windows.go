// +build windows

package checks

import (
	"bytes"
	"runtime"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/shirou/w32"
	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
	process "github.com/DataDog/gopsutil/process"
)

var (
	modpsapi                  = windows.NewLazyDLL("psapi.dll")
	modkernel                 = windows.NewLazyDLL("kernel32.dll")
	procGetProcessMemoryInfo  = modpsapi.NewProc("GetProcessMemoryInfo")
	procGetProcessHandleCount = modkernel.NewProc("GetProcessHandleCount")
	procGetProcessIoCounters  = modkernel.NewProc("GetProcessIoCounters")

	// XXX: Cross-check state is stored globally so the checks are not thread-safe.
	cachedProcesses = map[uint32]*cachedProcess{}
	// cacheProcessesMutex is a mutex to protect cachedProcesses from being accessed concurrently.
	// So far this is the case for Process check and RTProcess check
	// TODO: revisit cacheProcesses usage so that we don't need to lock the whole getAllProcesses()
	cacheProcessesMutex sync.Mutex
	checkCount          = 0
	haveWarnedNoArgs    = false
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

func getAllProcStats(probe *procutil.Probe, pids []int32) (map[int32]*procutil.Stats, error) {
	procs, err := getAllProcesses(probe)
	if err != nil {
		return nil, err
	}
	stats := make(map[int32]*procutil.Stats, len(procs))
	for pid, proc := range procs {
		stats[pid] = proc.Stats
	}
	return stats, nil
}

func getAllProcesses(probe *procutil.Probe) (map[int32]*procutil.Process, error) {
	// make sure we get the consistent snapshot by using the same OS thread
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	allProcsSnap := w32.CreateToolhelp32Snapshot(w32.TH32CS_SNAPPROCESS, 0)
	if allProcsSnap == 0 {
		return nil, windows.GetLastError()
	}
	procs := make(map[int32]*procutil.Process)

	defer w32.CloseHandle(allProcsSnap)
	var pe32 w32.PROCESSENTRY32
	pe32.DwSize = uint32(unsafe.Sizeof(pe32))

	checkCount++

	cacheProcessesMutex.Lock()
	defer cacheProcessesMutex.Unlock()

	knownPids := make(map[uint32]struct{})
	for pid := range cachedProcesses {
		knownPids[pid] = struct{}{}
	}

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
			cp = &cachedProcess{}

			if err := cp.fillFromProcEntry(&pe32); err != nil {
				log.Debugf("could not fill Win32 process information for pid %v %v", pid, err)
				continue
			}
			cachedProcesses[pid] = cp
		} else {
			if err := cp.openProcHandle(pe32.Th32ProcessID); err != nil {
				log.Debugf("Could not reopen process handle for pid %v %v", pid, err)
				continue
			}
		}
		defer cp.close()

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
		procs[int32(pid)] = &procutil.Process{
			Pid:     int32(pid),
			Ppid:    int32(ppid),
			Cmdline: cp.parsedArgs,
			Stats: &procutil.Stats{
				CreateTime:  ctime,
				OpenFdCount: int32(handleCount),
				NumThreads:  int32(pe32.CntThreads),
				CPUTime: &procutil.CPUTimesStat{
					User:      utime,
					System:    stime,
					Timestamp: time.Now().UnixNano(),
				},
				MemInfo: &procutil.MemoryInfoStat{
					RSS:  uint64(pmemcounter.WorkingSetSize),
					VMS:  uint64(pmemcounter.QuotaPagedPoolUsage),
					Swap: 0,
				},
				IOStat: &procutil.IOCountersStat{
					ReadCount:  int64(ioCounters.ReadOperationCount),
					WriteCount: int64(ioCounters.WriteOperationCount),
					ReadBytes:  int64(ioCounters.ReadTransferCount),
					WriteBytes: int64(ioCounters.WriteTransferCount),
				},
				CtxSwitches: &procutil.NumCtxSwitchesStat{},
			},

			Exe:      cp.executablePath,
			Username: cp.userName,
		}
	}
	for pid := range knownPids {
		cp := cachedProcesses[pid]
		log.Debugf("removing process %v %v", pid, cp.executablePath)
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

type cachedProcess struct {
	userName       string
	executablePath string
	commandLine    string
	procHandle     windows.Handle
	parsedArgs     []string
}

func (cp *cachedProcess) fillFromProcEntry(pe32 *w32.PROCESSENTRY32) (err error) {
	err = cp.openProcHandle(pe32.Th32ProcessID)
	if err != nil {
		return err
	}
	var usererr error
	cp.userName, usererr = getUsernameForProcess(cp.procHandle)
	if usererr != nil {
		log.Debugf("Couldn't get process username %v %v", pe32.Th32ProcessID, err)
	}
	var cmderr error
	cp.executablePath = winutil.ConvertWindowsString16(pe32.SzExeFile[:])
	cp.commandLine, cmderr = winutil.GetCommandLineForProcess(cp.procHandle)
	if cmderr != nil {
		log.Debugf("Error retrieving full command line %v", cmderr)
		cp.commandLine = cp.executablePath
	}

	cp.parsedArgs = parseCmdLineArgs(cp.commandLine)
	return
}

func (cp *cachedProcess) openProcHandle(pid uint32) (err error) {
	// 0x1000 is PROCESS_QUERY_LIMITED_INFORMATION, but that constant isn't
	//        defined in x/sys/windows
	// 0x10   is PROCESS_VM_READ

	cp.procHandle, err = windows.OpenProcess(0x1010, false, uint32(pid))
	if err != nil {
		log.Debugf("Couldn't open process with PROCESS_VM_READ %v %v", pid, err)
		cp.procHandle, err = windows.OpenProcess(0x1000, false, uint32(pid))
		if err != nil {
			log.Debugf("Couldn't open process %v %v", pid, err)
			return err
		}
	}
	return
}
func (cp *cachedProcess) close() {
	if cp.procHandle != windows.Handle(0) {
		windows.CloseHandle(cp.procHandle)
		cp.procHandle = windows.Handle(0)
	}
	return
}
