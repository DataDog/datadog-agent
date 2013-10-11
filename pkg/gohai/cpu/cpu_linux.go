package cpu

import (
	"io/ioutil"
	"regexp"
)

type Cpu struct{}

func (self *Cpu) Collect() (result map[string]string, err error) {
	re := regexp.MustCompile("model name\t: (.*)")
	cpuinfo, err := ioutil.ReadFile("/proc/cpuinfo")
	if err != nil { panic(err) }
	cpu := re.FindStringSubmatch(string(cpuinfo))[1]
	return map[string]string{
		"cpu": cpu,
	}, err
}
