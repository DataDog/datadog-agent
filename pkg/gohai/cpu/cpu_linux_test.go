package cpu

import (
	"fmt"
	. "github.com/r7kamura/gospel"
	"testing"
	"io/ioutil"
	"regexp"
)

func TestCpu(t *testing.T) {
	Describe(t, "cpu.Collect", func() {
		collector := &Cpu{}
		result, _ := collector.Collect()

		re := regexp.MustCompile("model name\t: (.*)")
		cpuinfo, err := ioutil.ReadFile("/proc/cpuinfo")
		if err != nil { panic(err) }
		cpu := re.FindStringSubmatch(string(cpuinfo))[1]

		fmt.Println(cpu)

		It("should be able to collect hostname", func() {
			Expect(result["cpu"]).To(Equal, cpu)
		})
	})
}
