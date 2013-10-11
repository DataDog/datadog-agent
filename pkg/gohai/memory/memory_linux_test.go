package memory

import (
	. "github.com/r7kamura/gospel"
	"testing"
)

func TestMemory(t *testing.T) {
	Describe(t, "memory.Collect", func() {
		collector := &Memory{}
		result, _ := collector.Collect()

		It("should be able to collect total memory size", func() {
			Expect(result["memory"]["total"]).To(NotEqual, "")
		})
	})
}
