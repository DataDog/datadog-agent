package gops

import (
	"github.com/shirou/gopsutil/process"
)

func pickName(p *process.Process) (string, error) {
	return p.Name()
}
