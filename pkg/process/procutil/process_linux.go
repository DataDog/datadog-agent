// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package procutil

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// WorldReadable represents file permission that's world readable
	WorldReadable os.FileMode = 4
	// DefaultClockTicks is the default number of clock ticks per second
	// C.sysconf(C._SC_CLK_TCK)
	DefaultClockTicks = float64(100)
	// More than 5760 to work around https://golang.org/issue/24015.
	blockSize = 8192
)

var (
	// PageSize is the system's memory page size
	PageSize = uint64(os.Getpagesize())
)

type statusInfo struct {
	name        string
	status      string
	uids        []int32
	gids        []int32
	nspid       int32
	numThreads  int32
	memInfo     *MemoryInfoStat
	ctxSwitches *NumCtxSwitchesStat
}

type statInfo struct {
	ppid       int32
	createTime int64
	nice       int32
	flags      uint32
	cpuStat    *CPUTimesStat
}

// WithReturnZeroPermStats configures whether StatsWithPermByPID() returns StatsWithPerm that
// has zero values on all fields
func WithReturnZeroPermStats(enabled bool) Option {
	return func(p Probe) {
		if linuxProbe, ok := p.(*probe); ok {
			linuxProbe.returnZeroPermStats = enabled
		}
	}
}

// WithPermission configures if process collection should fetch fields
// that require elevated permission or not
func WithPermission(elevatedPermissions bool) Option {
	return func(p Probe) {
		if linuxProbe, ok := p.(*probe); ok {
			linuxProbe.elevatedPermissions = elevatedPermissions
		}
	}
}

// WithBootTimeRefreshInterval configures the boot time refresh interval
func WithBootTimeRefreshInterval(bootTimeRefreshInterval time.Duration) Option {
	return func(p Probe) {
		if linuxProbe, ok := p.(*probe); ok {
			linuxProbe.bootTimeRefreshInterval = bootTimeRefreshInterval
		}
	}
}

// probe is a service that fetches process related info on current host
type probe struct {
	bootTime     *atomic.Uint64
	procRootLoc  string // ProcFS
	procRootFile *os.File
	uid          uint32 // UID
	euid         uint32 // Effective UID
	clockTicks   float64
	exit         chan struct{}

	// configurations
	elevatedPermissions     bool
	returnZeroPermStats     bool
	bootTimeRefreshInterval time.Duration
}

// NewProcessProbe initializes a new Probe object
func NewProcessProbe(options ...Option) Probe {
	hostProc := util.HostProc()
	bootTime, err := bootTime(hostProc)
	if err != nil {
		log.Errorf("could not parse boot time: %s", err)
	}

	p := &probe{
		procRootLoc:             hostProc,
		uid:                     uint32(os.Getuid()),
		euid:                    uint32(os.Geteuid()),
		clockTicks:              getClockTicks(),
		exit:                    make(chan struct{}),
		bootTime:                atomic.NewUint64(0),
		bootTimeRefreshInterval: time.Minute,
	}
	p.bootTime.Store(bootTime)

	for _, o := range options {
		o(p)
	}

	go p.syncBootTime()

	return p
}

// Close cleans up everything related to Probe object
func (p *probe) Close() {
	close(p.exit)
	if p.procRootFile != nil {
		p.procRootFile.Close()
		p.procRootFile = nil
	}
}

// syncBootTime checks bootTime every minute and stores it.
// Make sure we get the correct boot time if the clock of the host is temporarily drifted but gets corrected later on
func (p *probe) syncBootTime() {
	ticker := time.NewTicker(p.bootTimeRefreshInterval)
	defer ticker.Stop()

	select {
	case <-ticker.C:
		bootTime, err := bootTime(p.procRootLoc)
		if err != nil {
			log.Errorf("could not parse boot time: %s", err)
		} else {
			p.bootTime.Store(bootTime)
		}
	case <-p.exit:
		return
	}
}

// StatsForPIDs returns a map of stats info indexed by PID using the given PIDs
func (p *probe) StatsForPIDs(pids []int32, now time.Time) (map[int32]*Stats, error) {
	statsByPID := make(map[int32]*Stats, len(pids))
	for _, pid := range pids {
		pathForPID := filepath.Join(p.procRootLoc, strconv.Itoa(int(pid)))
		if !util.PathExists(pathForPID) {
			log.Debugf("Unable to create new process %d, dir %s doesn't exist", pid, pathForPID)
			continue
		}

		statusInfo := p.parseStatus(pathForPID)
		statInfo := p.parseStat(pathForPID, pid, now)
		memInfoEx := p.parseStatm(pathForPID)

		stats := &Stats{
			CreateTime:  statInfo.createTime,    // /proc/[pid]/stat
			Status:      statusInfo.status,      // /proc/[pid]/status
			Nice:        statInfo.nice,          // /proc/[pid]/stat
			CPUTime:     statInfo.cpuStat,       // /proc/[pid]/stat
			MemInfo:     statusInfo.memInfo,     // /proc/[pid]/status
			MemInfoEx:   memInfoEx,              // /proc/[pid]/statm
			CtxSwitches: statusInfo.ctxSwitches, // /proc/[pid]/status
			NumThreads:  statusInfo.numThreads,  // /proc/[pid]/status
		}
		if p.elevatedPermissions {
			stats.OpenFdCount = p.getFDCount(pathForPID) // /proc/[pid]/fd, requires permission checks
			stats.IOStat = p.parseIO(pathForPID)         // /proc/[pid]/io, requires permission checks
		} else {
			stats.IOStat = &IOCountersStat{
				ReadCount:  -1,
				WriteCount: -1,
				ReadBytes:  -1,
				WriteBytes: -1,
			} // use -1 values to represent "no permission"
		}
		statsByPID[pid] = stats
	}
	return statsByPID, nil
}

// ProcessesByPID returns a map of process info indexed by PID
func (p *probe) ProcessesByPID(now time.Time, collectStats bool) (map[int32]*Process, error) {
	pids, err := p.getActivePIDs()
	if err != nil {
		return nil, err
	}

	procsByPID := make(map[int32]*Process, len(pids))
	for _, pid := range pids {
		pathForPID := filepath.Join(p.procRootLoc, strconv.Itoa(int(pid)))
		if !util.PathExists(pathForPID) {
			log.Debugf("Unable to create new process %d, dir %s doesn't exist", pid, pathForPID)
			continue
		}

		cmdline := p.getCmdline(pathForPID)
		statusInfo := p.parseStatus(pathForPID)
		statInfo := p.parseStat(pathForPID, pid, now)

		if len(cmdline) == 0 {
			if isKernelThread(statInfo.flags) {
				log.Tracef("Skipping kernel process pid:%d", pid)
				// NOTE: The agent's process check currently skips all processes that are kernel threads which have
				//       no cmdline and they have the PF_KTHREAD flag set in /proc/<pid>/stat
				//       Moving this check down the stack saves us from a number of needless follow-up system calls.
				continue
			}
			log.Debugf("process with empty cmdline not skipped pid:%d", pid)
		}

		// On linux, setting the `collectStats` parameter to false will only prevent collection of memory stats.
		// It does not prevent collection of stats from the /proc/(pid)/stat file, since we need to read the
		// createTime to make a bytekey
		var memInfoEx *MemoryInfoExStat
		if collectStats {
			memInfoEx = p.parseStatm(pathForPID)
		} else {
			memInfoEx = &MemoryInfoExStat{}
		}

		proc := &Process{
			Pid:     pid,                                       // /proc/[pid]
			Ppid:    statInfo.ppid,                             // /proc/[pid]/stat
			Cmdline: cmdline,                                   // /proc/[pid]/cmdline
			Name:    statusInfo.name,                           // /proc/[pid]/status
			Uids:    statusInfo.uids,                           // /proc/[pid]/status
			Gids:    statusInfo.gids,                           // /proc/[pid]/status
			Cwd:     p.getLinkWithAuthCheck(pathForPID, "cwd"), // /proc/[pid]/cwd, requires permission checks
			Exe:     p.getLinkWithAuthCheck(pathForPID, "exe"), // /proc/[pid]/exe, requires permission checks
			NsPid:   statusInfo.nspid,                          // /proc/[pid]/status
			Stats: &Stats{
				CreateTime:  statInfo.createTime,    // /proc/[pid]/stat
				Status:      statusInfo.status,      // /proc/[pid]/status
				Nice:        statInfo.nice,          // /proc/[pid]/stat
				CPUTime:     statInfo.cpuStat,       // /proc/[pid]/stat
				MemInfo:     statusInfo.memInfo,     // /proc/[pid]/status
				MemInfoEx:   memInfoEx,              // /proc/[pid]/statm
				CtxSwitches: statusInfo.ctxSwitches, // /proc/[pid]/status
				NumThreads:  statusInfo.numThreads,  // /proc/[pid]/status
			},
		}
		if p.elevatedPermissions {
			proc.Stats.OpenFdCount = p.getFDCount(pathForPID) // /proc/[pid]/fd, requires permission checks
			proc.Stats.IOStat = p.parseIO(pathForPID)         // /proc/[pid]/io, requires permission checks
		} else {
			proc.Stats.IOStat = &IOCountersStat{
				ReadCount:  -1,
				WriteCount: -1,
				ReadBytes:  -1,
				WriteBytes: -1,
			} // use -1 values to represent "no permission"
		}
		procsByPID[pid] = proc
	}

	return procsByPID, nil
}

// StatsWithPermByPID returns the stats that require elevated permission to collect for each process
func (p *probe) StatsWithPermByPID(pids []int32) (map[int32]*StatsWithPerm, error) {
	statsByPID := make(map[int32]*StatsWithPerm, len(pids))
	for _, pid := range pids {
		pathForPID := filepath.Join(p.procRootLoc, strconv.Itoa(int(pid)))
		if !util.PathExists(pathForPID) {
			log.Debugf("Unable to create new process %d, dir %s doesn't exist", pid, pathForPID)
			continue
		}

		fds := p.getFDCount(pathForPID)
		io := p.parseIO(pathForPID)

		// don't return entries with all zero values if returnZeroPermStats is disabled
		if !p.returnZeroPermStats && fds == 0 && io.IsZeroValue() {
			continue
		}

		statsByPID[pid] = &StatsWithPerm{
			OpenFdCount: fds,
			IOStat:      io,
		}
	}
	return statsByPID, nil
}

func (p *probe) getRootProcFile() (*os.File, error) {
	if p.procRootFile != nil {
		return p.procRootFile, nil
	}

	f, err := os.Open(p.procRootLoc)
	if err == nil {
		p.procRootFile = f
	}

	return f, err
}

// getActivePIDs retrieves a list of PIDs representing actively running processes.
func (p *probe) getActivePIDs() ([]int32, error) {
	procFile, err := p.getRootProcFile()
	if err != nil {
		return nil, err
	}

	fnames, err := procFile.Readdirnames(-1)
	if err != nil {
		return nil, err
	}

	// reset read offset to 0, so next time we could read the whole directory again
	_, err = procFile.Seek(0, 0)
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

// getCmdline retrieves the command line text from "cmdline" file for a process in procfs
func (p *probe) getCmdline(pidPath string) []string {
	cmdline, err := os.ReadFile(filepath.Join(pidPath, "cmdline"))
	if err != nil {
		log.Debugf("Unable to read process command line from %s: %s", pidPath, err)
		return nil
	}

	if len(cmdline) == 0 {
		return nil
	}

	return trimAndSplitBytes(cmdline)
}

// parseIO retrieves io info from "io" file for a process in procfs
func (p *probe) parseIO(pidPath string) *IOCountersStat {
	path := filepath.Join(pidPath, "io")
	var err error

	io := &IOCountersStat{
		ReadBytes:  -1,
		ReadCount:  -1,
		WriteBytes: -1,
		WriteCount: -1,
	}

	if err = p.ensurePathReadable(path); err != nil {
		return io
	}

	f, err := os.Open(path)
	if err != nil {
		return io
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		p.parseIOLine(scanner.Bytes(), io)
	}
	return io
}

// parseIOLine extracts key and value for each line in "io" file
func (p *probe) parseIOLine(line []byte, io *IOCountersStat) {
	for i := range line {
		// the fields are all having format "field_name: field_value", so we always
		// look for ": " and skip them
		if i+2 < len(line) && line[i] == ':' && unicode.IsSpace(rune(line[i+1])) {
			key := line[0:i]
			value := line[i+2:]
			p.parseIOKV(string(key), string(value), io)
			break
		}
	}
}

// parseIOKV matches key with a field in IOCountersStat model and fills in the value
func (p *probe) parseIOKV(key, value string, io *IOCountersStat) {
	switch key {
	case "syscr":
		v, err := strconv.ParseInt(value, 10, 64)
		if err == nil {
			io.ReadCount = v
		}
	case "syscw":
		v, err := strconv.ParseInt(value, 10, 64)
		if err == nil {
			io.WriteCount = v
		}
	case "read_bytes":
		v, err := strconv.ParseInt(value, 10, 64)
		if err == nil {
			io.ReadBytes = v
		}
	case "write_bytes":
		v, err := strconv.ParseInt(value, 10, 64)
		if err == nil {
			io.WriteBytes = v
		}
	}
}

// parseStatus retrieves status info from "status" file for a process in procfs
func (p *probe) parseStatus(pidPath string) *statusInfo {
	path := filepath.Join(pidPath, "status")
	var err error

	sInfo := &statusInfo{
		uids:        []int32{},
		gids:        []int32{},
		memInfo:     &MemoryInfoStat{},
		ctxSwitches: &NumCtxSwitchesStat{},
	}

	content, err := os.ReadFile(path)

	if err != nil {
		return sInfo
	}

	lineStart := 0
	for i, r := range content {
		if r == '\n' {
			p.parseStatusLine(content[lineStart:i], sInfo)
			lineStart = i + 1
		}
	}

	return sInfo
}

// parseStatusLine takes each line in "status" file and parses info from it
func (p *probe) parseStatusLine(line []byte, sInfo *statusInfo) {
	for i := range line {
		// the fields are all having format "field_name:\s+field_value", so we always
		// look for ":\t" and skip them
		if i+2 < len(line) && line[i] == ':' && unicode.IsSpace(rune(line[i+1])) {
			key := line[0:i]
			value := line[i+2:]
			p.parseStatusKV(string(key), string(value), sInfo)
			break
		}
	}
}

// parseStatusKV takes tokens parsed from each line in "status" file and populates statusInfo object
func (p *probe) parseStatusKV(key, value string, sInfo *statusInfo) {
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
	case "NSpid":
		values := strings.Split(value, "\t")
		// only report process namespaced PID
		v, err := strconv.ParseInt(values[len(values)-1], 10, 32)
		if err == nil {
			sInfo.nspid = int32(v)
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
		value := strings.TrimSuffix(value, "kB") // trim spaces and suffix "kB"
		value = strings.Trim(value, " ")
		v, err := strconv.ParseUint(value, 10, 64)
		if err == nil {
			sInfo.memInfo.RSS = v * 1024
		}
	case "VmSize":
		value := strings.TrimSuffix(value, "kB") // trim spaces and suffix "kB"
		value = strings.Trim(value, " ")
		v, err := strconv.ParseUint(value, 10, 64)
		if err == nil {
			sInfo.memInfo.VMS = v * 1024
		}
	case "VmSwap":
		value := strings.TrimSuffix(value, "kB") // trim spaces and suffix "kB"
		value = strings.Trim(value, " ")
		v, err := strconv.ParseUint(value, 10, 64)
		if err == nil {
			sInfo.memInfo.Swap = v * 1024
		}
	}
}

// parseStat retrieves stat info from "stat" file for a process in procfs
func (p *probe) parseStat(pidPath string, pid int32, now time.Time) *statInfo {
	path := filepath.Join(pidPath, "stat")
	var err error

	sInfo := &statInfo{
		cpuStat: &CPUTimesStat{},
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		return sInfo
	}

	sInfo = p.parseStatContent(contents, sInfo, pid, now)
	return sInfo
}

// parseStatContent takes the content of "stat" file and parses the values we care about
func (p *probe) parseStatContent(statContent []byte, sInfo *statInfo, pid int32, now time.Time) *statInfo {
	// We want to skip past the executable name, which is wrapped in one or more parenthesis
	startIndex := bytes.LastIndexByte(statContent, byte(')'))
	if startIndex == -1 || startIndex+1 >= len(statContent) {
		return sInfo
	}

	content := statContent[startIndex+1:]
	// use spaces and prevCharIsSpace to simulate strings.Fields() to avoid allocation
	spaces := 0
	prevCharIsSpace := false
	var ppidStr, flagStr, utimeStr, stimeStr, startTimeStr string

	for _, c := range content {
		if unicode.IsSpace(rune(c)) {
			if !prevCharIsSpace {
				spaces++
			}
			prevCharIsSpace = true
			continue
		} else {
			prevCharIsSpace = false
		}

		switch spaces {
		case 2:
			ppidStr += string(c)
		case 7:
			flagStr += string(c)
		case 12:
			utimeStr += string(c)
		case 13:
			stimeStr += string(c)
		case 20:
			startTimeStr += string(c)
		}
	}

	if spaces < 20 { // We access index 20 and below, so this is just a safety check.
		return sInfo
	}

	ppid, err := strconv.ParseInt(ppidStr, 10, 32)
	if err == nil {
		sInfo.ppid = int32(ppid)
	}

	flags, err := strconv.ParseInt(flagStr, 10, 32)
	if err == nil {
		sInfo.flags = uint32(flags)
	}

	utime, err := strconv.ParseFloat(utimeStr, 64)
	if err == nil {
		sInfo.cpuStat.User = utime / p.clockTicks
	}

	stime, err := strconv.ParseFloat(stimeStr, 64)
	if err == nil {
		sInfo.cpuStat.System = stime / p.clockTicks
	}
	// the nice parameter location seems to be different for various procfs,
	// so we fetch that using syscall
	snice, err := syscall.Getpriority(syscall.PRIO_PROCESS, int(pid))
	if err == nil {
		sInfo.nice = int32(snice)
	}

	sInfo.cpuStat.Timestamp = now.Unix()

	t, err := strconv.ParseUint(startTimeStr, 10, 64)
	if err == nil {
		ctime := (t / uint64(p.clockTicks)) + p.bootTime.Load()
		// convert create time into milliseconds
		sInfo.createTime = int64(ctime * 1000)
	}

	return sInfo
}

// parseStatm gets memory info from /proc/(pid)/statm
func (p *probe) parseStatm(pidPath string) *MemoryInfoExStat {
	path := filepath.Join(pidPath, "statm")
	var err error

	memInfoEx := &MemoryInfoExStat{}

	contents, err := os.ReadFile(path)
	if err != nil {
		return memInfoEx
	}

	fields := strings.Fields(string(contents))

	// the values for the fields are per-page, to get real numbers we multiply by PageSize
	vms, err := strconv.ParseUint(fields[0], 10, 64)
	if err == nil {
		memInfoEx.VMS = vms * PageSize
	}
	rss, err := strconv.ParseUint(fields[1], 10, 64)
	if err == nil {
		memInfoEx.RSS = rss * PageSize
	}
	shared, err := strconv.ParseUint(fields[2], 10, 64)
	if err == nil {
		memInfoEx.Shared = shared * PageSize
	}
	text, err := strconv.ParseUint(fields[3], 10, 64)
	if err == nil {
		memInfoEx.Text = text * PageSize
	}
	lib, err := strconv.ParseUint(fields[4], 10, 64)
	if err == nil {
		memInfoEx.Lib = lib * PageSize
	}
	data, err := strconv.ParseUint(fields[5], 10, 64)
	if err == nil {
		memInfoEx.Data = data * PageSize
	}
	dirty, err := strconv.ParseUint(fields[6], 10, 64)
	if err == nil {
		memInfoEx.Dirty = dirty * PageSize
	}

	return memInfoEx
}

// getLinkWithAuthCheck fetches the destination of a symlink with permission check
func (p *probe) getLinkWithAuthCheck(pidPath string, file string) string {
	path := filepath.Join(pidPath, file)
	if err := p.ensurePathReadable(path); err != nil {
		return ""
	}

	str, err := os.Readlink(path)
	if err != nil {
		return ""
	}
	return str
}

// getFDCount gets num_fds from /proc/(pid)/fd WITHOUT using the native Readdirnames(),
// this will skip the step of returning all file names(we don't need) in a dir which takes a lot of memory
func (p *probe) getFDCount(pidPath string) int32 {
	path := filepath.Join(pidPath, "fd")

	if err := p.ensurePathReadable(path); err != nil {
		return -1
	}

	d, err := os.Open(path)
	if err != nil {
		return -1
	}
	defer d.Close()

	b := make([]byte, blockSize)
	count := 0

	for i := 0; ; i++ {
		n, err := syscall.ReadDirent(int(d.Fd()), b)
		if err != nil {
			return -1
		}
		if n <= 0 {
			break
		}

		_, numDirs := countDirent(b[:n])
		count += numDirs
	}

	return int32(count)
}

// ensurePathReadable ensures that the current user is able to read the path before opening it.
// On some systems, attempting to open a file that the user does not have permission is problematic for
// customer security auditing. What we do here is:
// 1. If the agent is running as root (real or via sudo), allow the request
// 2. If the file is a not a symlink and has the other-readable permission bit set, allow the request
// 3. If the owner of the file/link is the current user or effective user, allow the request.
func (p *probe) ensurePathReadable(path string) error {
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

// trimAndSplitBytes converts the raw command line bytes into a list of strings by trimming and splitting on null bytes
func trimAndSplitBytes(bs []byte) []string {
	var components []string

	// Remove leading null bytes
	i := 0
	for i < len(bs) {
		if bs[i] == 0 {
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

	// attach the last segment if the string is not ended with null byte
	if i < len(bs) {
		components = append(components, string(bs[i:]))
	}

	return components
}

// bootTime returns the system boot time expressed in seconds since the epoch.
// the value is extracted from "/proc/stat"
func bootTime(hostProc string) (uint64, error) {
	filePath := filepath.Join(hostProc, "stat")
	content, err := os.ReadFile(filePath)
	if err != nil {
		return 0, fmt.Errorf("unable to read stat file from %s: %s", filePath, err)
	}

	lineStart := 0
	btimePrefix := []byte("btime")

	for i, r := range content {
		if r == '\n' {
			if bytes.HasPrefix(content[lineStart:i], btimePrefix) {
				f := strings.Fields(string(content[lineStart:i]))
				if len(f) != 2 {
					return 0, fmt.Errorf("wrong btime format: %s", content[lineStart:i])
				}

				b, err := strconv.ParseInt(f[1], 10, 64)
				if err != nil {
					return 0, err
				}
				return uint64(b), nil
			}
			lineStart = i + 1
		}
	}

	return 0, fmt.Errorf("btime data does not exist in %s", filePath)
}

// getClockTicks uses command "getconf CLK_TCK" to fetch the clock tick on current host,
// if the command doesn't exist uses the default value 100
func getClockTicks() float64 {
	clockTicks := DefaultClockTicks
	getconf, err := exec.LookPath("getconf")
	if err != nil {
		return clockTicks
	}
	cmd := exec.Command(getconf, "CLK_TCK")
	var out bytes.Buffer
	cmd.Stdout = &out
	err = cmd.Run()
	if err == nil {
		ticks, err := strconv.ParseFloat(strings.TrimSpace(out.String()), 64)
		if err == nil {
			clockTicks = ticks
		}
	}
	return clockTicks
}

// isKernelThread checks if the PF_KTHREAD flag is set which identifies this process as a kernel thread
// See: https://github.com/torvalds/linux/commit/7b34e4283c685f5cc6ba6d30e939906eee0d4bcf
func isKernelThread(flags uint32) bool {
	return flags&0x00200000 == 0x00200000
}
