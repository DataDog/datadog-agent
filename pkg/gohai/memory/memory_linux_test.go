package memory

import (
	. "github.com/r7kamura/gospel"
	"testing"
)

func TestMemory(t *testing.T) {
	Describe(t, "memory.Name", func() {
		collector := &Memory{}

		It("should have its own name", func() {
			Expect(collector.Name()).To(Equal, "memory")
		})
	})

	Describe(t, "memory.Collect", func() {
		collector := &Memory{}
		result, _ := collector.Collect()

		It("should be able to collect total memory size", func() {
			Expect(result.(map[string]string)["total"]).To(NotEqual, "")
		})
	})
}
