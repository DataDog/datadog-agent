// +build !windows

package procdiscovery

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"path"
	"strconv"

	goproc "github.com/shirou/gopsutil/process"
)

func pollProcesses() ([]process, error) {
	processes, err := goproc.Processes()

	if err != nil {
		return nil, fmt.Errorf("Couldn't retrieve process list: %s", err)
	}

	pl := make([]process, 0, len(processes))

	for _, p := range processes {
		cmd, err := p.Cmdline()
		// Just ignore the process if we can't get its cmdline
		if err != nil {
			continue
		}

		// Do not retrieve processes running inside containers
		if is, err := isContainer(p.Pid); !is && err == nil {
			pl = append(pl, process{pid: p.Pid, cmd: cmd})
		}
	}

	return pl, nil
}

// Hold cgroup names used by common containers systems
var cgroupNames = [][]byte{[]byte("docker")}

func isContainer(pid int32) (bool, error) {
	// Read the /proc/$PID/cgroup file to retrieve cgroups
	raw, err := ioutil.ReadFile(path.Join("/proc", strconv.Itoa(int(pid)), "cgroup"))
	if err != nil {
		return false, fmt.Errorf("error reading cgroup file for process with pid %d", pid)
	}

	for _, n := range cgroupNames {
		if bytes.Contains(raw, n) {
			return true, nil
		}
	}

	return false, nil
}
