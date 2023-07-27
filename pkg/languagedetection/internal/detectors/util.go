package detectors

import (
	"path"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/process/util"
)

func getExePath(pid int) string {
	hostProc := util.HostProc()
	return path.Join(hostProc, strconv.Itoa(pid), "exe")
}
