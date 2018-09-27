// +build !windows

package procdiscovery

import (
	"fmt"

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

		pl = append(pl, process{pid: p.Pid, cmd: cmd})
	}

	return pl, nil
}
