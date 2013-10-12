package cpu

import (
	"bufio"
	"os"
	"regexp"
	"strconv"
)

type Cpu struct{}

const name = "cpu"

func (self *Cpu) Name() string {
    return name
}

func (self *Cpu) Collect() (result interface{}, err error) {
	result, err = getcpuInfo()
	return
}

func getcpuInfo() (cpuInfo map[string]string, err error) {
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
	count := 0

	for _, line := range lines {
		pair := regexp.MustCompile("\t: ").Split(line, 2)

		switch pair[0] {
		case "processor":
			cpuInfo["processor"] = pair[1]
			count += 1
		case "vendor_id":
			cpuInfo["vendor_id"] = pair[1]
		case "family":
			cpuInfo["family"] = pair[1]
		case "model":
			cpuInfo["model"] = pair[1]
		case "model name":
			cpuInfo["model_name"] = pair[1]
		case "stepping":
			cpuInfo["stepping"] = pair[1]
		case "physical id":
			cpuInfo["physical_id"] = pair[1]
		case "core id":
			cpuInfo["physical_id"] = pair[1]
		case "cpu cores":
			cpuInfo["cpu cores"] = pair[1]
		case "cpu MHz":
			cpuInfo["mhz"] = pair[1]
		case "cache size":
			cpuInfo["cache_size"] = pair[1]
		case "flags":
			cpuInfo["flags"] = pair[1]
		}
	}

	cpuInfo["total"] = strconv.Itoa(count)
	return
}
