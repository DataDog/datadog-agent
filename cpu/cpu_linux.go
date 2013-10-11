package cpu

import (
	"io/ioutil"
	"strings"
	"regexp"
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

	lines := strings.Split(string(contents), "\n")

	cpuinfo = make(map[string]string)

	for _, line := range(lines) {
		fields := regSplit(line, "\t+: ")
		switch fields[0] {
		case "model name": cpuinfo["model_name"] = fields[1]
		}
	}
	return
}

func regSplit(text string, delimeter string) []string {
	reg := regexp.MustCompile(delimeter)
	indexes := reg.FindAllStringIndex(text, -1)
	laststart := 0
	result := make([]string, len(indexes) + 1)
	for i, element := range indexes {
		result[i] = text[laststart:element[0]]
		laststart = element[1]
	}
	result[len(indexes)] = text[laststart:len(text)]
	return result
}
