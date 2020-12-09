// +build linux

package procutil

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/gopsutil/host"
)

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

		procsByPID[pid] = &Process{
			Pid:     pid,     // /proc/{pid}
			Ppid:    0,       // /proc/{pid}/stat
			Cmdline: cmdline, // /proc/{pid}/cmdline
		}
	}

	return procsByPID, nil
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

// getCmdline takes file path for a PID and retrieves the command line text
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

func (p *Probe) getRootProcFile() (*os.File, error) {
	if p.procRootFile != nil { // TODO (sk): Should we consider refreshing the file descriptor occasionally?
		return p.procRootFile, nil
	}

	f, err := os.Open(p.procRootLoc)
	if err != nil {
		return nil, err
	}

	p.procRootFile = f
	return f, nil
}
