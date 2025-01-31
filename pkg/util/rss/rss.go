package rss

import (
	"io/ioutil"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"github.com/dustin/go-humanize"
	"github.com/shirou/gopsutil/v4/process"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var pageSize = syscall.Getpagesize()

// rss returns the resident set size of the current process, unit in MiB
func rss() string {
	data, err := ioutil.ReadFile("/proc/self/stat")
	if err != nil {
		return ""
	}
	fs := strings.Fields(string(data))
	rss, err := strconv.ParseInt(fs[23], 10, 64)
	if err != nil {
		return ""
	}
	b := uint64(uintptr(rss) * uintptr(pageSize) / (1 << 20))
	return humanize.Bytes(b) // MiB
}

func getPs(pid int) string {
	var value string
	ps, _ := process.Processes()
	for _, p := range ps {
		if p.Pid == int32(pid) {
			mem, _ := p.MemoryInfo()
			value = humanize.Bytes(mem.RSS)
			break
		}
	}

	return value
}

func Before(name string) {
	m := runtime.MemStats{}
	runtime.ReadMemStats(&m)
	log.Infof("RSS INFORMATION BEFORE %s: %s, rss(%s), ps(%s)", name, humanize.Bytes(m.Sys), rss(), getPs(syscall.Getpid()))
}

func After(name string) {
	m := runtime.MemStats{}
	runtime.ReadMemStats(&m)
	log.Infof("RSS INFORMATION AFTER %s: %s, rss(%s), ps(%s)", name, humanize.Bytes(m.Sys), rss(), getPs(syscall.Getpid()))
}
