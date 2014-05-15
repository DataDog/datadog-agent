package memory

import (
	"os/exec"
	"regexp"
	"strings"
)

func getMemoryInfo() (memoryInfo map[string]string, err error) {
	memoryInfo = make(map[string]string)

	out, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
	if err != nil {
		return memoryInfo, err
	}
	memoryInfo["total"] = strings.Trim(string(out), "\n")

	out, err = exec.Command("sysctl", "-n", "vm.swapusage").Output()
	if err != nil {
		return memoryInfo, err
	}
	swap := regexp.MustCompile("total = ").Split(string(out), 2)[1]
	memoryInfo["swap_total"] = strings.Split(swap, " ")[0]

	return
}
