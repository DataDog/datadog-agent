// +build linux

package procutil

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/gopsutil/host"
)

type statusInfo struct {
	name       string
	status     string
	uids       []int32
	gids       []int32
	nspid      int32
	numThreads int32

	memInfo     *MemoryInfoStat
	ctxSwitches *NumCtxSwitchesStat
}

// Probe is a service that fetches process related info on current host
type Probe struct {
	procRootLoc  string // ProcFS
	procRootFile *os.File

	uid  uint32 // Used for path permission checking to prevent access to files that we can't access
	euid uint32

	bootTime uint64
}

// NewProcessProbe initializes a new Probe object
func NewProcessProbe() *Probe {
	bootTime, _ := host.BootTime() // TODO (sk): Rewrite this w/o gopsutil

	p := &Probe{
		procRootLoc: util.HostProc(),
		uid:         uint32(os.Getuid()),
		euid:        uint32(os.Geteuid()),
		bootTime:    bootTime,
	}
	return p
}

// Close cleans up everything related to Probe object
func (p *Probe) Close() {
	if p.procRootFile != nil {
		p.procRootFile.Close()
		p.procRootFile = nil
	}
}

// ProcessesByPID returns a map of process info indexed by PID
func (p *Probe) ProcessesByPID() (map[int32]*Process, error) {
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
		if len(cmdline) == 0 {
			// NOTE: The agent's process check currently skips all processes that have no cmdline (i.e kernel processes).
			//       Moving this check down the stack saves us from a number of needless follow-up system calls.
			continue
		}

		statusInfo := p.parseStatus(pathForPID)

		procsByPID[pid] = &Process{
			Pid:     pid,               // /proc/{pid}
			Cmdline: cmdline,           // /proc/{pid}/cmdline
			Name:    statusInfo.name,   // /proc/{pid}/status
			Status:  statusInfo.status, // /proc/{pid}/status
			Uids:    statusInfo.uids,   // /proc/{pid}/status
			Gids:    statusInfo.gids,   // /proc/{pid}/status
			NsPid:   statusInfo.nspid,  // /proc/{pid}/status
			Stats: &Stats{
				MemInfo:     statusInfo.memInfo,     // /proc/{pid}/status or statm
				CtxSwitches: statusInfo.ctxSwitches, // /proc/{pid}/status
				NumThreads:  statusInfo.numThreads,  // /proc/{pid}/status
			},
		}
	}

	return procsByPID, nil
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

// getActivePIDs retrieves a list of PIDs representing actively running processes.
func (p *Probe) getActivePIDs() ([]int32, error) {
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
func (p *Probe) getCmdline(pidPath string) []string {
	cmdline, err := ioutil.ReadFile(filepath.Join(pidPath, "cmdline"))
	if err != nil {
		log.Debugf("Unable to read process command line from %s: %s", pidPath, err)
		return nil
	}

	if len(cmdline) == 0 {
		return nil
	}

	return trimAndSplitBytes(cmdline)
}

// parseStatus retrieves status info from "status" file for a process in procfs
func (p *Probe) parseStatus(pidPath string) *statusInfo {
	path := filepath.Join(pidPath, "status")
	var err error

	sInfo := &statusInfo{
		uids:        []int32{},
		gids:        []int32{},
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

// parseStatusLine takes each line in "status" file and parses info from it
func (p *Probe) parseStatusLine(line []byte, sInfo *statusInfo) {
	for i := range line {
		// the fields are all having format "field_name:\tfield_value", so we always
		// look for ":\t" and skip them
		if i+2 < len(line) && line[i] == ':' && line[i+1] == '\t' {
			key := line[0:i]
			value := line[i+2:]
			p.parseStatusKV(string(key), string(value), sInfo)
			break
		}
	}
}

// parseStatusKV takes tokens parsed from each line in "status" file and populates statusInfo object
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
