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
	result, err = getCpuInfo()
	return
}

var cpuMap = map[string]string{
	"vendor_id":  "vendor_id",
	"model name": "model_name",
	"cpu cores":  "cpu_cores",
	"cpu MHz\t":  "mhz",
	"cache size": "cache_size",
	"processor":   "processor",
	"cpu family":  "family",
	"model\t":       "model",
	"stepping":    "stepping",
	"physical id": "physical_id",
}

// fixme tri This collector saves only the last processor's info

func getCpuInfo() (cpuInfo map[string]interface{}, err error) {
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

	cpuInfo = make(map[string]interface{})
	count := 1
	currentCPU := make(map[string]string)

	for _, line := range lines[1:] {
		pair := regexp.MustCompile("\t: ").Split(line, 2)

		key, ok := cpuMap[pair[0]]
		if pair[0] == "processor" { // next processor's information
			cpuInfo[strconv.Itoa(count)] = currentCPU
			count += 1
		}
		if ok {
			currentCPU[key] = pair[1]
		}
	}
	cpuInfo[strconv.Itoa(count)] = currentCPU

	return
}
