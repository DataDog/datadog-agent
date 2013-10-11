package cpu

import (
	"io/ioutil"
	"regexp"
	"strings"
)

type Cpu struct{}

func (self *Cpu) Collect() (result map[string]map[string]string, err error) {
	cpuinfo, err := getCpuInfo()

	return map[string]map[string]string{
		"cpu": cpuinfo,
	}, err
}

func getCpuInfo() (cpuinfo map[string]string, err error) {
	contents, err := ioutil.ReadFile("/proc/cpuinfo")

	if err != nil {
		return
	}

	cpuinfo = make(map[string]string)
    count := 0

	lines := strings.Split(string(contents), "\n")

	for _, line := range lines {
		fields := regSplit(line, "\t+: ")
		switch fields[0] {
		case "processor":
			cpuinfo["processor"] = fields[1]
			count += 1
		case "vendor_id":
			cpuinfo["vendor_id"] = fields[1]
		case "family":
			cpuinfo["family"] = fields[1]
		case "model":
			cpuinfo["model"] = fields[1]
		case "model name":
			cpuinfo["model_name"] = fields[1]
		case "stepping":
			cpuinfo["stepping"] = fields[1]
		case "physical id":
			cpuinfo["physical_id"] = fields[1]
		case "core id":
			cpuinfo["physical_id"] = fields[1]
		case "cpu cores":
			cpuinfo["cpu cores"] = fields[1]
		case "cpu MHz":
			cpuinfo["mhz"] = fields[1]
		case "cache size":
			cpuinfo["cache_size"] = fields[1]
		case "flags":
			cpuinfo["flags"] = fields[1]
		}
	}

	cpuinfo["total"] = string(count)
	return
}

func regSplit(text string, delimeter string) []string {
	reg := regexp.MustCompile(delimeter)
	indexes := reg.FindAllStringIndex(text, -1)
	laststart := 0
	result := make([]string, len(indexes)+1)

	for i, element := range indexes {
		result[i] = text[laststart:element[0]]
		laststart = element[1]
	}

	result[len(indexes)] = text[laststart:len(text)]
	return result
}
