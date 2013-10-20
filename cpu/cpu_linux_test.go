package cpu

import (
	. "github.com/r7kamura/gospel"
	"testing"
)

func TestCpu(t *testing.T) {
	Describe(t, "cpu.Name()", func() {
		collector := &Cpu{}

		It("should have its name", func() {
			Expect(collector.Name()).To(Equal, "cpu")
		})
	})

	Describe(t, "cpu.Collect()", func() {
		collector := &Cpu{}
		result, _ := collector.Collect()

		It("should be able to collect CPU model name", func() {
			Expect(result.(map[string]string)["model_name"]).To(NotEqual, "")
		})

		It("should be able to collect the number of CPU(s)", func() {
			Expect(result.(map[string]string)["total"]).To(NotEqual, "0")
		})
	})
}
