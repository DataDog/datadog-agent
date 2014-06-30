package cpu

import (
	"bufio"
	"os"
	"regexp"
	"strconv"
)

var cpuMap = map[string]string{
	"vendor_id":  "vendor_id",
	"model name": "model_name",
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

	// count # of occurences to get # of cores, since the field
	// "cpu cores" isn't reliable
	numCores := 0

	for _, line := range lines[1:] {
		pair := regexp.MustCompile("\t: ").Split(line, 2)

		key, ok := cpuMap[pair[0]]
		if ok {
			cpuInfo[key] = pair[1]
			if pair[0] == "vendor_id" {
				numCores += 1
			}
		}
	}
	cpuInfo["cpu_cores"] = strconv.Itoa(numCores)

	return
}
