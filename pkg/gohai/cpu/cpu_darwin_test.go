package cpu

import (
	"debug/macho"
	"fmt"
	. "github.com/r7kamura/gospel"
	"testing"
)

func TestCpu(t *testing.T) {
	Describe(t, "cpu.Collect", func() {
		collector := &Cpu{}
		result, _ := collector.Collect()
		cpu := (&macho.FileHeader{}).Cpu
		fmt.Println(cpu.String())

		It("should be able to collect cpu", func() {
			Expect(result["cpu"]).To(Equal, cpu.String())
		})
	})
}
