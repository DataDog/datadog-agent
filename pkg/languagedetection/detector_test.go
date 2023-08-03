package languagedetection

import (
	"math/rand"

	"github.com/DataDog/datadog-agent/pkg/process/procutil"
)

func makeProcess(cmdline []string, comm string) *procutil.Process {
	return &procutil.Process{
		Pid:     rand.Int31(),
		Cmdline: cmdline,
		Comm:    comm,
	}
}
