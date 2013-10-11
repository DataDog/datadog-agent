package cpu

import (
	"bufio"
	"os"
	"regexp"
)

type Cpu struct{}

func (self *Cpu) Collect() (result map[string]map[string]string, err error) {
	cpuinfo, err := getCpuInfo()
	result = map[string]map[string]string{
		"cpu": cpuinfo,
	}

	return
}

func getCpuInfo() (cpuinfo map[string]string, err error) {
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

	cpuinfo = make(map[string]string)
	count := 0

	for _, line := range lines {
		pair := regexp.MustCompile("\t: ").Split(line, 2)

		switch pair[0] {
		case "processor":
			cpuinfo["processor"] = pair[1]
			count += 1
		case "vendor_id":
			cpuinfo["vendor_id"] = pair[1]
		case "family":
			cpuinfo["family"] = pair[1]
		case "model":
			cpuinfo["model"] = pair[1]
		case "model name":
			cpuinfo["model_name"] = pair[1]
		case "stepping":
			cpuinfo["stepping"] = pair[1]
		case "physical id":
			cpuinfo["physical_id"] = pair[1]
		case "core id":
			cpuinfo["physical_id"] = pair[1]
		case "cpu cores":
			cpuinfo["cpu cores"] = pair[1]
		case "cpu MHz":
			cpuinfo["mhz"] = pair[1]
		case "cache size":
			cpuinfo["cache_size"] = pair[1]
		case "flags":
			cpuinfo["flags"] = pair[1]
		}
	}

	cpuinfo["total"] = string(count)
	return
}
