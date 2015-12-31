package cpu

import (
	"bufio"
	"os"
	"regexp"
)

var cpuMap = map[string]string{
	"vendor_id":  "vendor_id",
	"model name": "model_name",
	"cpu cores":  "cpu_cores",
	"siblings":   "cpu_logical_processors",
	"cpu MHz\t":  "mhz",
	"cache size": "cache_size",
	"cpu family": "family",
	"model\t":    "model",
	"stepping":   "stepping",
}

func getCpuInfo() (cpuInfo map[string]string, err error) {
	file, err := os.Open("/proc/cpuinfo")

	if err != nil {
		return
	}

	var lines []string
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if scanner.Err() != nil {
		err = scanner.Err()
		return
	}

	cpuInfo = make(map[string]string)

	for _, line := range lines[1:] {
		pair := regexp.MustCompile("\t: ").Split(line, 2)

		key, ok := cpuMap[pair[0]]
		if ok {
			cpuInfo[key] = pair[1]
		}
	}

	return
}
