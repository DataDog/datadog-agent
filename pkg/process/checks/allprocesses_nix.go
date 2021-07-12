// +build !windows

package checks

import (
	"github.com/DataDog/gopsutil/process"
	"github.com/StackVista/stackstate-agent/pkg/process/config"
)

func getAllProcesses(*config.AgentConfig) (map[int32]*process.FilledProcess, error) {
	return process.AllProcesses()
}
