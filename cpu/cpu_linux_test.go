package cpu

import (
	. "github.com/r7kamura/gospel"
	"testing"
)

func TestCpu(t *testing.T) {
	Describe(t, "cpu.Collect", func() {
		collector := &Cpu{}
		result, _ := collector.Collect()

		It("should be able to collect CPU model name", func() {
			Expect(result["cpu"]["model_name"]).To(NotEqual, "")
		})

		It("should be able to collect the number of CPU(s)", func() {
			Expect(result["cpu"]["total"]).To(NotEqual, "0")
		})
	})
}
