package cpu

import (
	"fmt"
	. "github.com/r7kamura/gospel"
	"testing"
)

func TestCpu(t *testing.T) {
	Describe(t, "cpu.Collect", func() {
		collector := &Cpu{}
		result, _ := collector.Collect()
		fmt.Println(result["cpu"]["model_namea"], ":")

		It("should be able to collect cpu model name", func() {
			Expect(result["cpu"]["model_name"]).To(NotEqual, "")
		})
	})
}
