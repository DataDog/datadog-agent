// +build linux

package process

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/DataDog/gopsutil/host"
	"github.com/DataDog/datadog-agent/pkg/process/procutil/internal/common"
)

var (
	PageSize = uint64(os.Getpagesize())
)

const (
	PrioProcess               = 0 // linux/resource.h
	WorldReadable os.FileMode = 4
	ClockTicks                = 100 // C.sysconf(C._SC_CLK_TCK)
)

type statInfo struct {
	ppid       int32
	createTime int64
	nice       int32

	cpuStat *CPUTimesStat
}

type statusInfo struct {
	name       string
	status     string
	uids       []int32
	gids       []int32
	numThreads int32

	memInfo     *MemoryInfoStat
	ctxSwitches *NumCtxSwitchesStat
}

type Probe struct {
	procRootLoc  string // ProcFS
	procRootFile *os.File

	uid  uint32 // Used for path permission checking to prevent access to files that we can't access
	euid uint32

	bootTime uint64
}

func NewProcessProbe() *Probe {
	bootTime, _ := host.BootTime() // TODO (sk): Rewrite this w/o gopsutil

	p := &Probe{
		procRootLoc: common.HostProc(),
		uid:         uint32(os.Getuid()),
		euid:        uint32(os.Geteuid()),
		bootTime:    bootTime,
	}
	return p
}

func (p *Probe) Close() {
	if p.procRootFile != nil {
		p.procRootFile.Close()
		p.procRootFile = nil
	}
}

func (p *Probe) ProcessesByPID() (map[int32]*Process, error) {
	pids, err := p.getActivePIDs()
	if err != nil {
		return nil, err
	}

	fmt.Println(pids)

	now := time.Now()

	procsByPID := make(map[int32]*Process, len(pids))
	for _, pid := range pids {
		pathForPID := filepath.Join(p.procRootLoc, strconv.Itoa(int(pid)))
		if !common.DoesDirExist(pathForPID) {
			// log.Debugf("Unable to create new process %d, it may have gone away: %s", pid, err)
			continue
		}

		cmdline := p.getCmdline(pathForPID)
		if len(cmdline) == 0 {
			// Note: The agent's process check currently skips all processes that have no cmdline (i.e kernel processes).
			//       Moving this check down the stack saves us from a number of needless follow-up system calls.
			//
			//       In the test resources for Postgres, this accounts for ~30% of processes.
			continue
		}

		statusInfo := p.parseStatus(pathForPID)
		ioInfo := p.parseIO(pathForPID)
		_, memInfoEx := p.parseStatm(pathForPID) // We're ignoring memInfo here since it doesn't have Swap stats
		statInfo := p.parseStat(pathForPID, int(pid), now)

		procsByPID[pid] = &Process{
			Pid:     pid,                                       // /proc/{pid}
			Ppid:    0,                                         // /proc/{pid}/stat
			Cmdline: cmdline,                                   // /proc/{pid}/cmdline
			Name:    statusInfo.name,                           // /proc/{pid}/status
			Uids:    statusInfo.uids,                           // /proc/{pid}/status
			Gids:    statusInfo.gids,                           // /proc/{pid}/status
			Cwd:     p.getLinkWithAuthCheck(pathForPID, "cwd"), // /proc/{pid}/cwd, requires permission checks
			Exe:     p.getLinkWithAuthCheck(pathForPID, "exe"), // /proc/{pid}/exe, requires permission checks
			Stats: &Stats{
				Status:      statusInfo.status,        // /proc/{pid}/status
				CreateTime:  statInfo.createTime,      // /proc/{pid}/{stat}
				Nice:        statInfo.nice,            // /proc/{pid}/{stat}
				OpenFdCount: p.getFDCount(pathForPID), // /proc/{pid}/fd, requires permission checks
				CpuTime:     statInfo.cpuStat,         // /proc/{pid}/{stat}
				IOStat:      ioInfo,                   // /proc/{pid}/io, requires permission checks
				MemInfoEx:   memInfoEx,                // /proc/{pid}/statm
				MemInfo:     statusInfo.memInfo,       // /proc/{pid}/status or statm
				CtxSwitches: statusInfo.ctxSwitches,   // /proc/{pid}/status
				NumThreads:  statusInfo.numThreads,    // /proc/{pid}/status
			},
		}
	}

	return procsByPID, nil
}

func (p *Probe) ProcessStatsForPIDs(pids []int32) (map[int32]*Stats, error) {
	now := time.Now()

	statsByPID := make(map[int32]*Stats, len(pids))
	for _, pid := range pids {
		pathForPID := filepath.Join(p.procRootLoc, strconv.Itoa(int(pid)))
		if !common.DoesDirExist(pathForPID) {
			// log.Debugf("Unable to create new process %d, it may have gone away: %s", pid, err)
			continue
		}

		statusInfo := p.parseStatus(pathForPID)
		ioInfo := p.parseIO(pathForPID)
		_, memInfoEx := p.parseStatm(pathForPID) // We're ignoring memInfo here since it doesn't have Swap stats
		statInfo := p.parseStat(pathForPID, int(pid), now)

		statsByPID[pid] = &Stats{
			Status:      statusInfo.status,        // /proc/{pid}/status
			CreateTime:  statInfo.createTime,      // /proc/{pid}/{stat}
			Nice:        statInfo.nice,            // /proc/{pid}/{stat}
			OpenFdCount: p.getFDCount(pathForPID), // /proc/{pid}/fd, requires permission checks
			CpuTime:     statInfo.cpuStat,         // /proc/{pid}/{stat}
			IOStat:      ioInfo,                   // /proc/{pid}/io
			MemInfoEx:   memInfoEx,                // /proc/{pid}/statm
			MemInfo:     statusInfo.memInfo,       // /proc/{pid}/status or statm
			CtxSwitches: statusInfo.ctxSwitches,   // /proc/{pid}/status
			NumThreads:  statusInfo.numThreads,    // /proc/{pid}/status
		}
	}
	return statsByPID, nil
}

// Retrieve a list of IDs representing actively running processes.
func (p *Probe) getActivePIDs() ([]int32, error) {
	procFile, err := p.getRootProcFile()
	if err != nil {
		return nil, err
	}

	fnames, err := procFile.Readdirnames(-1)
	if err != nil {
		return nil, err
	}

	pids := make([]int32, 0, len(fnames))
	for _, fname := range fnames {
		pid, err := strconv.ParseInt(fname, 10, 32)
		if err != nil { // if not numeric name, just skip
			continue
		}
		pids = append(pids, int32(pid))
	}

	return pids, nil
}

func (p *Probe) getCmdline(pidPath string) []string {
	cmdline, err := ioutil.ReadFile(filepath.Join(pidPath, "cmdline"))
	if err != nil {
		// log.Debugf("Unable to read process command line for %d: %s", pid, err)
		return nil
	}

	if len(cmdline) == 0 {
		return nil
	}

	return trimAndSplitBytes(cmdline)
}

// TODO (sk): Optimize this
func (p *Probe) parseIO(pidPath string) *IOCountersStat {
	path := filepath.Join(pidPath, "io")
	var err error

	defer func() {
		logErrorForPath(path, err)
	}()

	io := &IOCountersStat{}

	if err = p.ensurePathReadable(path); err != nil {
		return io
	}

	ioline, err := ioutil.ReadFile(path)
	if err != nil {
		return io
	}

	lines := strings.Split(string(ioline), "\n")
	for _, line := range lines {
		field := strings.Fields(line)
		if len(field) < 2 {
			continue
		}

		t, err := strconv.ParseUint(field[1], 10, 64)
		if err != nil {
			continue
		}

		param := field[0]
		if strings.HasSuffix(param, ":") {
			param = param[:len(param)-1]
		}

		switch param {
		case "syscr":
			io.ReadCount = t
		case "syscw":
			io.WriteCount = t
		case "read_bytes":
			io.ReadBytes = t
		case "write_bytes":
			io.WriteBytes = t
		}
	}

	return io
}

func (p *Probe) parseStatus(pidPath string) *statusInfo {
	path := filepath.Join(pidPath, "status")
	var err error

	defer func() {
		logErrorForPath(path, err)
	}()

	sInfo := &statusInfo{
		uids:        make([]int32, 0),
		gids:        make([]int32, 0),
		memInfo:     &MemoryInfoStat{},
		ctxSwitches: &NumCtxSwitchesStat{},
	}

	contents, err := ioutil.ReadFile(path)
	if err != nil {
		return sInfo
	}

	index := 0
	for i, r := range contents {
		if r == '\n' {
			p.parseStatusLine(contents[index:i], sInfo)
			index = i + 1
		}
	}

	return sInfo
}

func (p *Probe) parseStatusLine(line []byte, sInfo *statusInfo) {
	for i := range line {
		if i+2 < len(line) && line[i] == ':' && line[i+1] == '\t' {
			key := line[0:i]
			value := line[i+2:]
			p.parseStatusKV(string(key), string(value), sInfo)
			break
		}
	}
}

func (p *Probe) parseStatusKV(key, value string, sInfo *statusInfo) {
	switch key {
	case "Name":
		sInfo.name = strings.Trim(value, " \t")
	case "State":
		sInfo.status = value[0:1]
	case "Uid":
		sInfo.uids = make([]int32, 0, 4)
		for _, i := range strings.Split(value, "\t") {
			v, err := strconv.ParseInt(i, 10, 32)
			if err == nil {
				sInfo.uids = append(sInfo.uids, int32(v))
			}
		}
	case "Gid":
		sInfo.gids = make([]int32, 0, 4)
		for _, i := range strings.Split(value, "\t") {
			v, err := strconv.ParseInt(i, 10, 32)
			if err == nil {
				sInfo.gids = append(sInfo.gids, int32(v))
			}
		}
	case "Threads":
		v, err := strconv.ParseInt(value, 10, 32)
		if err == nil {
			sInfo.numThreads = int32(v)
		}
	case "voluntary_ctxt_switches":
		v, err := strconv.ParseInt(value, 10, 64)
		if err == nil {
			sInfo.ctxSwitches.Voluntary = v
		}
	case "nonvoluntary_ctxt_switches":
		v, err := strconv.ParseInt(value, 10, 64)
		if err == nil {
			sInfo.ctxSwitches.Involuntary = v
		}
	case "VmRSS":
		value := strings.Trim(value, " kB") // remove last "kB"
		v, err := strconv.ParseUint(value, 10, 64)
		if err == nil {
			sInfo.memInfo.RSS = v * 1024
		}
	case "VmSize":
		value := strings.Trim(value, " kB") // remove last "kB"
		v, err := strconv.ParseUint(value, 10, 64)
		if err == nil {
			sInfo.memInfo.VMS = v * 1024
		}
	case "VmSwap":
		value := strings.Trim(value, " kB") // remove last "kB"
		v, err := strconv.ParseUint(value, 10, 64)
		if err == nil {
			sInfo.memInfo.Swap = v * 1024
		}
	}
}

func (p *Probe) parseStat(pidPath string, pid int, now time.Time) *statInfo {
	path := filepath.Join(pidPath, "stat")
	var err error

	defer func() {
		logErrorForPath(path, err)
	}()

	sInfo := &statInfo{
		cpuStat: &CPUTimesStat{},
	}

	contents, err := ioutil.ReadFile(path)
	if err != nil {
		return sInfo
	}

	startIndex := bytes.IndexByte(contents, byte(')')) // We want to skip past the executable name, which is wrapped in parenthesis
	if startIndex+1 >= len(contents) {
		return sInfo
	}

	fields := strings.Fields(string(contents[startIndex+1:]))
	if 20 >= len(fields) { // We access index 20 and below, so this is just a safety check.
		return sInfo
	}

	ppid, err := strconv.ParseInt(fields[2], 10, 32)
	if err == nil {
		sInfo.ppid = int32(ppid)
	}

	utime, err := strconv.ParseFloat(fields[12], 64)
	if err != nil {
		return sInfo
	}
	stime, err := strconv.ParseFloat(fields[13], 64)
	if err != nil {
		return sInfo
	}

	sInfo.cpuStat.CPU = "cpu"
	sInfo.cpuStat.User = float64(utime / ClockTicks)
	sInfo.cpuStat.System = float64(stime / ClockTicks)
	sInfo.cpuStat.Timestamp = now.Unix()

	t, err := strconv.ParseUint(fields[20], 10, 64)
	if err != nil {
		return sInfo
	}

	ctime := (t / uint64(ClockTicks)) + uint64(p.bootTime)
	sInfo.createTime = int64(ctime * 1000)

	// use syscall instead of parse Stat file
	snice, _ := syscall.Getpriority(PrioProcess, pid)
	sInfo.nice = int32(snice)

	return sInfo
}

// Get memory info from /proc/(pid)/statm
func (p *Probe) parseStatm(pidPath string) (*MemoryInfoStat, *MemoryInfoExStat) {
	path := filepath.Join(pidPath, "statm")
	var err error

	defer func() {
		logErrorForPath(path, err)
	}()

	memInfo := &MemoryInfoStat{}
	memInfoEx := &MemoryInfoExStat{}

	contents, err := ioutil.ReadFile(path)
	if err != nil {
		return memInfo, memInfoEx
	}
	fields := strings.Split(string(contents), " ")

	vms, err := strconv.ParseUint(fields[0], 10, 64)
	if err != nil {
		return memInfo, memInfoEx
	}
	rss, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return memInfo, memInfoEx
	}

	memInfo.RSS = rss * PageSize
	memInfo.VMS = vms * PageSize

	shared, err := strconv.ParseUint(fields[2], 10, 64)
	if err != nil {
		return memInfo, memInfoEx
	}
	text, err := strconv.ParseUint(fields[3], 10, 64)
	if err != nil {
		return memInfo, memInfoEx
	}
	lib, err := strconv.ParseUint(fields[4], 10, 64)
	if err != nil {
		return memInfo, memInfoEx
	}
	dirty, err := strconv.ParseUint(fields[5], 10, 64)
	if err != nil {
		return memInfo, memInfoEx
	}

	memInfoEx.RSS = rss * PageSize
	memInfoEx.VMS = vms * PageSize
	memInfoEx.Shared = shared * PageSize
	memInfoEx.Text = text * PageSize
	memInfoEx.Lib = lib * PageSize
	memInfoEx.Dirty = dirty * PageSize

	return memInfo, memInfoEx
}

// Get num_fds from /proc/(pid)/fd
func (p *Probe) getFDCount(pidPath string) int32 {
	path := filepath.Join(pidPath, "fd")
	var err error

	defer func() {
		logErrorForPath(path, err)
	}()

	if err = p.ensurePathReadable(path); err != nil {
		return 0
	}

	d, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer d.Close()

	names, err := d.Readdirnames(-1)
	return int32(len(names))
}

func (p *Probe) getLinkWithAuthCheck(pidPath string, file string) string {
	path := filepath.Join(pidPath, file)
	var err error
	var str string

	defer func() {
		logErrorForPath(path, err)
	}()

	if err = p.ensurePathReadable(path); err != nil {
		return ""
	}

	str, err = os.Readlink(path)
	return string(str)
}

// TODO (sk): Write test(s) for this - test cases below
// [40 115 100 45 112 97 109 41 0 0 0]
// [[40 115 100 45 112 97 109 41]]
//
// [115 115 104 100 58 32 118 97 103 114 97 110 116 64 112 116 115 47 48 0 0]
// [[115 115 104 100 58 32 118 97 103 114 97 110 116 64 112 116 115 47 48]]
//
// [47 117 115 114 47 98 105 110 47 100 111 99 107 101 114 100 0 45 72 0 102 100 58 47 47 0]
// [[47 117 115 114 47 98 105 110 47 100 111 99 107 101 114 100] [45 72] [102 100 58 47 47]]
//
// [0 0 47 115 98 105 110 47 105 110 105 116 0]
// [[47 115 98 105 110 47 105 110 105 116]]
func trimAndSplitBytes(bs []byte) []string {
	var components []string

	// Remove leading null bytes
	i := 0
	for j := 0; j < len(bs); j++ {
		if bs[j] == 0 {
			i++
		} else {
			break
		}
	}

	// Split our stream using the null byte separator
	for j := i; j < len(bs); j++ {
		if bs[j] == 0 {
			components = append(components, string(bs[i:j]))
			i = j + 1

			// If we have successive null bytes, skip them (this will also remove trailing null characters)
			for i < len(bs) && bs[i] == 0 {
				i++
				j++
			}
		}
	}

	return components
}

func (p *Probe) getRootProcFile() (*os.File, error) {
	if p.procRootFile != nil { // TODO (sk): Should we consider refreshing the file descriptor occasionally?
		return p.procRootFile, nil
	}

	f, err := os.Open(p.procRootLoc)
	if err == nil {
		p.procRootFile = f
	}

	return f, err
}

// ensurePathReadable ensures that the current user is able to read the path in question
// before opening it. On some systems, attempting to open a file that the user does
// not have permission is problematic for customer security auditing
func (p *Probe) ensurePathReadable(path string) error {
	// User is (effectively or actually) root
	if p.euid == 0 {
		return nil
	}

	// TODO (sk): Provide caching on this!
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}

	// File mode is world readable and not a symlink
	// If the file is a symlink, the owner check below will cover it
	if mode := info.Mode(); mode&os.ModeSymlink == 0 && mode.Perm()&WorldReadable != 0 {
		return nil
	}

	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		// If file is not owned by the user id or effective user id, return a permission error
		// Group permissions don't come in to play with procfs so we don't bother checking
		if stat.Uid != p.uid && stat.Uid != p.euid {
			return os.ErrPermission
		}
	}

	return nil
}

func logErrorForPath(path string, err error) {
	if err != nil {
		//if os.IsPermission(err) {
		//	log.Tracef("Unable to access %s, permission denied", path)
		//} else {
		//	log.Debugf("Unable to access %s: %s", path, err)
		//}
		//fmt.Printf("%s - %s\n", path, err)
	}
}
