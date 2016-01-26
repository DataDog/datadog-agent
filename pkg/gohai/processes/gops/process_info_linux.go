package gops

import (
	"strings"

	"github.com/shirou/gopsutil/process"
)

func pickName(p *process.Process) (string, error) {
	cmdline, err := p.Cmdline()
	if err != nil {
		return "", err
	}
	// Assume that the process is a kernel process when it has no args
	if strings.TrimSpace(cmdline) == "" {
		return "kernel", nil
	}

	return p.Name()
}
